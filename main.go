package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v2"

	_ "embed"
)

//go:embed default-config.yml
var config []byte

/*
TODO:
Usage: cliview [--config|-c CONFIG] [file|dir|url|-]
*/
func main() {
	log.SetFlags(0)

	conf, err := loadConfig()
	if err != nil {
		log.Println(err)
		os.Exit(1)
	}

	arg := os.Args[1]

	configs, err := parseManyToOneConfigs(conf.FileTypes)
	if err != nil {
		log.Println(err)
		os.Exit(1)
	}

	if isFile(arg) {
		if n, err := handleFile(configs, arg); err != nil {
			log.Println(err)
			os.Exit(n)
		} else {
			os.Exit(0)
		}
	}

	log.Println("No preview available for:\n", arg)
	os.Exit(1)
}

type Config struct {
	FileTypes yaml.MapSlice `yaml:"file_types"`
}

type MIME struct {
	Type    string
	Subtype string
	Value   string
}

func parseMIME(s string) MIME {
	parts := strings.Split(s, "/")
	typ := strings.TrimSpace(parts[0])
	if len(parts) == 1 {
		return MIME{
			Type:    typ,
			Subtype: "",
			Value:   typ,
		}
	}
	sub := strings.TrimSpace(parts[1])
	return MIME{
		Type:    typ,
		Subtype: sub,
		Value:   typ + "/" + sub,
	}
}

// parseManyToOneConfigs is a helper function to turn the configs into
// an iterable list of command instructions.
// Parses keys as a list of comma-separated strings and
// maps each one to the command specified by the value
func parseManyToOneConfigs(cmds yaml.MapSlice) ([][]string, error) {
	var list [][]string
	for _, item := range cmds {
		types, ok := item.Key.(string)
		if !ok {
			return nil, fmt.Errorf("config key must be a string: %v", item.Key)
		}
		command, ok := item.Value.(string)
		if !ok {
			return nil, fmt.Errorf("config value must be a string: %v", item.Value)
		}
		typs := strings.Split(types, ",")
		for _, t := range typs {
			t = strings.Trim(t, " ")
			list = append(list, []string{t, command})
		}
	}
	return list, nil
}

// Will return true if arg is a directory, file, or symlink
func isFile(arg string) bool {
	_, err := os.Lstat(arg)
	return err == nil
}

func isMimeType(mime string) bool {
	return strings.Count(mime, "/") == 1
}

func isExtension(ext string) bool {
	return strings.HasPrefix(ext, ".")
}

func handleFile(configs [][]string, path string) (int, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return 0, err
	}
	ext := filepath.Ext(path)

	// Use the `file` command to determine the mime type of the
	output, err := exec.Command("file", "-b", "--mime-type", abs).Output()
	if err != nil {
		return 0, err
	}

	log.Println(string(output))

	fileMIME := parseMIME(string(output))

	for _, item := range configs {
		typ, cmd := item[0], item[1]

		switch {
		case isMimeType(typ):
			confMIME := parseMIME(typ)
			if confMIME.Type == "*" || confMIME.Type == fileMIME.Type {
				if confMIME.Subtype == "*" || confMIME.Subtype == fileMIME.Subtype {
					return eval(cmd, abs)
				}
			}
		case isExtension(typ):
			if typ == ext {
				return eval(cmd, abs)
			}
		}
	}

	// if we get here, we didn't find a match, so fallthrough to default handling
	return handleDefault(configs, abs)
}

func handleDefault(configs [][]string, arg string) (int, error) {
	for _, item := range configs {
		typ, cmd := item[0], item[1]
		if typ == "default" {
			return eval(cmd, arg)
		}
	}
	return 0, nil
}

func eval(cmd string, arg string) (int, error) {
	cmd = strings.ReplaceAll(cmd, "{}", arg)
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "sh"
	}

	c := exec.Command(shell, "-c", cmd)
	c.Stderr = os.Stderr
	c.Stdout = os.Stdout
	if err := c.Run(); err != nil {
		log.Println("error executing command:", cmd)
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode(), err
		}
		return 1, err
	}
	return 0, nil
}

func maybeWriteDefaultConfig() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	if _, err := os.Stat(home + "/.cliview/config.yml"); !os.IsNotExist(err) {
		// if the file already exists, don't overwrite it
		return nil
	}
	if err := os.MkdirAll(home+"/.cliview", 0755); err != nil {
		return err
	}
	f, err := os.Create(home + "/.cliview/config.yml")
	if err != nil {
		return err
	}
	if _, err := f.Write(config); err != nil {
		return err
	}
	return nil
}

func loadConfig() (*Config, error) {
	if err := maybeWriteDefaultConfig(); err != nil {
		return nil, err
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	configYML, err := os.Open(homeDir + "/.cliview/config.yml")
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
