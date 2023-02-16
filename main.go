package main

import (
	"fmt"
	"io"
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
	maybeWriteDefaultConfig()
	conf := loadConfig()

	arg := os.Args[1]

	switch {
	case isFile(arg):
		handleFile(conf, arg)
	}
	fmt.Println("No view available for:\n", arg)
}

type Config struct {
	Mimes      yaml.MapSlice
	Extensions yaml.MapSlice
	Default    string
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

func isFile(arg string) bool {
	_, err := os.Stat(arg)
	return !os.IsNotExist(err)
}

func handleFile(conf Config, arg string) {
	abs := do(filepath.Abs(arg))
	ext := filepath.Ext(arg)
	if ext != "" {
		handleFileByExtension(conf, abs, ext)
		// if we get here, we didn't find a match, so fallthrough to mime handling
	}
	handleFileByMime(conf, abs)
	// if we get here, we didn't find a match, so fallthrough to default handling
	handleDefault(conf, abs)
}

func handleFileByExtension(conf Config, arg string, ext string) {
	for _, item := range conf.Extensions {
		exts, cmd := item.Key.(string), item.Value.(string)
		ee := strings.Split(exts, ",")
		for _, e := range ee {
			e = strings.Trim(e, " ")
			if e == ext {
				exitCode := eval(cmd, arg)
				os.Exit(exitCode)
			}
		}
	}
}

func handleFileByMime(conf Config, arg string) {
	// Use the `file` command to determine the mime type
	output := do(exec.Command("file", "-b", "--mime-type", arg).Output())
	mimeType := parseMIME(string(output))

	for _, item := range conf.Mimes {
		mimes, cmd := item.Key.(string), item.Value.(string)
		mm := strings.Split(mimes, ",")
		for _, m := range mm {
			// valid forms: "image/*", "image/jpeg", "video/mp4,video/quicktime"
			m = strings.Trim(m, " ")
			confType := parseMIME(m)
			if confType.Type == "*" || confType.Type == mimeType.Type {
				if confType.Subtype == "*" || confType.Subtype == mimeType.Subtype {
					exitCode := eval(cmd, arg)
					os.Exit(exitCode)
				}
			}
		}
	}
}

func handleDefault(conf Config, arg string) {
	exitCode := eval(conf.Default, arg)
	os.Exit(exitCode)
}

func eval(cmd string, arg string) int {
	cmd = strings.ReplaceAll(cmd, "{}", arg)
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "sh"
	}
	c := exec.Command(shell, "-c", cmd)
	c.Stderr = os.Stderr
	c.Stdout = os.Stdout
	if err := c.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode()
		}
		panic(err)
	}
	return 0
}

func maybeWriteDefaultConfig() {
	home := do(os.UserHomeDir())
	_, err := os.Stat(home + "/.cliview/config.yml")
	if !os.IsNotExist(err) {
		return
	}
	if err := os.MkdirAll(home+"/.cliview", 0755); err != nil {
		panic(err)
	}
	f := do(os.Create(home + "/.cliview/config.yml"))
	n := do(f.Write(config))
	if n != len(config) {
		panic("failed to write config")
	}
}

func loadConfig() Config {
	home := do(os.UserHomeDir())
	configYML := do(os.Open(home + "/.cliview/config.yml"))
	var conf Config
	if err := yaml.Unmarshal(do(io.ReadAll(configYML)), &conf); err != nil {
		fmt.Printf("%+v\n", conf)
		panic(err)
	}
	return conf
}

func replaceArg(arg string, cmd string) string {
	return strings.ReplaceAll(cmd, "{}", arg)
}

// do is a generic function that will accept any type T and an error,
// handle the error, then return T alone.
func do[T any](t T, err error) T {
	if err != nil {
		panic(err)
	}
	return t
}
