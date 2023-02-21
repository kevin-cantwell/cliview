package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/gobwas/glob"
	"github.com/urfave/cli"
	"gopkg.in/yaml.v2"

	_ "embed"
)

var (
	//go:embed default-config.yml
	config []byte
)

func main() {
	log.SetFlags(0)

	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Println("Unable to determine home directory")
		os.Exit(1)
	}

	app := cli.NewApp()
	app.Version = "0.1.0"
	app.Name = "cliview"
	app.Usage = "Preview any file directly in the terminal."
	app.UsageText = "cliview [options] FILE"
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "config,c",
			Usage: "The config file to use.",
			Value: filepath.Join(homeDir, ".config", "cliview", "config.yml"),
		},
		cli.BoolFlag{
			Name:  "explain,e",
			Usage: "Print the configured view command without executing it.",
		},
	}
	app.Action = func(c *cli.Context) error {
		arg := c.Args().First()
		if arg == "" {
			return fmt.Errorf("No FILE specified")
		}
		explain := c.Bool("explain")
		configs, err := loadConfig(c.String("config"))
		if err != nil {
			return err
		}

		cmd := strings.Join(configs.Classifiers, " ;")
		buf := bytes.Buffer{}
		if _, err := eval(cmd, arg, &buf); err != nil {
			return err
		}
		classifications := strings.Split(string(buf.Bytes()), "\n")

		// Select and execute a viewer command by calculating the classification
		// of the arg and matching it against the glob patterns in the config.
		for _, viewer := range configs.Viewers {
			for _, classification := range classifications {
				g, err := glob.Compile(viewer.Classification)
				if err != nil {
					return err
				}
				if g.Match(classification) {
					if explain {
						fmt.Println(strings.ReplaceAll(viewer.Command, "{}", arg))
						return nil
					} else {
						eval(viewer.Command, arg)
						return err
					}
				}
			}
		}

		log.Printf("No preview available for:\n%s\n", arg)
		return nil
	}

	if err := app.Run(os.Args); err != nil {
		log.Println(err)
		os.Exit(1)
	}
}

type Config struct {
	Classifiers []string `yaml:"classifiers"`
	Viewers     Viewers  `yaml:"viewers"`
}

type Viewer struct {
	Classification string
	Command        string
}

type Viewers []Viewer

func (v *Viewers) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var viewers yaml.MapSlice
	if err := unmarshal(&viewers); err != nil {
		return err
	}
	*v = make(Viewers, 0)
	for _, item := range viewers {
		classifications, ok := item.Key.(string)
		if !ok {
			return fmt.Errorf("config key must be a string: %v", item.Key)
		}
		command, ok := item.Value.(string)
		if !ok {
			return fmt.Errorf("config value must be a string: %v", item.Value)
		}
		typs := strings.Split(classifications, ",")
		for _, typ := range typs {
			typ = strings.TrimSpace(typ)
			*v = append(*v, Viewer{
				Classification: typ,
				Command:        command,
			})
		}
	}

	return nil
}

func eval(cmd string, arg string, stdout ...io.Writer) (int, error) {
	cmd = strings.ReplaceAll(cmd, "{}", arg)
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "sh"
	}
	c := exec.Command(shell, "-c", cmd)
	c.Stderr = os.Stderr
	if len(stdout) > 0 {
		c.Stdout = stdout[0]
	} else {
		c.Stdout = os.Stdout
	}
	if err := c.Run(); err != nil {
		log.Println("error executing command:", cmd)
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode(), err
		}
		return 1, err
	}
	return 0, nil
}

func maybeWriteDefaultConfig(configPath string) error {
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		// if the file already exists, don't overwrite it
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return err
	}
	f, err := os.Create(configPath)
	if err != nil {
		return err
	}
	if _, err := f.Write(config); err != nil {
		return err
	}
	return nil
}

func loadConfig(configPath string) (*Config, error) {
	if err := maybeWriteDefaultConfig(configPath); err != nil {
		return nil, err
	}
	configYML, err := os.Open(configPath)
	if err != nil {
		return nil, err
	}
	bb, err := io.ReadAll(configYML)
	if err != nil {
		return nil, err
	}
	var conf Config
	if err := yaml.Unmarshal(bb, &conf); err != nil {
		return nil, err
	}
	return &conf, nil
}
