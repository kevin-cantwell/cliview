package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"github.com/h2non/filetype"
)

func main() {
	fname := os.Args[1]
	abs := do(filepath.Abs(fname))

	var exitCode int

	f := do(os.Open(abs))
	defer func() {
		f.Close()
		os.Exit(exitCode)
	}()

	if do(f.Stat()).IsDir() {
		exitCode = execer("exa", "-T", abs)
		return
	}

	r := bufio.NewReader(f)
	b, err := r.Peek(512)
	if err != nil && err != io.EOF {
		fmt.Println(err)
		exitCode = 1
		return
	}
	mimeType := do(filetype.Match(b))
	ext := filepath.Ext(abs)

	fmt.Printf("%s: %s\n", ext, mimeType.MIME.Value)

	switch {
	case mimeType.MIME.Type == "image":
		switch mimeType.MIME.Subtype {
		case "gif":
			// only allow .01s of animation for gifs, since fzf preview is not interactive
			exitCode = execer("timg", "-g", "80x400", "-t", ".01", abs)
		default:
			exitCode = execer("timg", "-g", "80x400", abs)
		}
	case mimeType.MIME.Type == "video":
		// only capture 1st frame since since fzf preview is not interactive
		exitCode = execer("timg", "-g", "80x40", "--frames", "1", "-V", abs)
	case mimeType.Extension == "sqlite":
		exitCode = piper(
			[]string{"sqlite3", abs, ".tables"},
			[]string{"tr", "-s", " ", `\n`},
			[]string{"xargs", "-I{}", "sqlite3", "-cmd", "select char(10) || '{}:'", "-cmd", ".mode column", abs, "pragma table_info('{}')"},
		)
	default:
		switch ext {
		case ".md":
			exitCode = execer("mdcat", abs)
		default:
			// test if this is a binary file
			exitCode = execer("grep", "-E", `\x00`, abs)
			if exitCode == 0 {
				// if so, display all non-printable characters
				exitCode = execer("bat", "-nA", "--color=always", abs)
			} else {
				// if not, display only printable characters
				exitCode = execer("bat", "-n", "--color=always", abs)
			}
		}
	}
}

func execer(args ...string) int {
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode()
		}
		panic(err)
	}
	return 0
}

func piper(argss ...[]string) int {
	if len(argss) == 0 {
		return 0
	}
	if len(argss) == 1 {
		return execer(argss[0]...)
	}
	var wg sync.WaitGroup
	defer wg.Wait()

	cmd := exec.Command(argss[0][0], argss[0][1:]...)
	cmd.Stderr = os.Stderr
	pipe := do(cmd.StdoutPipe())
	if err := cmd.Start(); err != nil {
		panic(err)
	}
	wg.Add(1)
	go func() {
		cmd.Wait()
		wg.Done()
	}()
	for _, args := range argss[1:] {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Stderr = os.Stderr
		cmd.Stdin = pipe
		pipe = do(cmd.StdoutPipe())
		if err := cmd.Start(); err != nil {
			panic(err)
		}
		wg.Add(1)
		go func() {
			cmd.Wait()
			wg.Done()
		}()
	}
	if _, err := io.Copy(os.Stdout, pipe); err != nil {
		panic(err)
	}
	return 0
}

// do is a generic function that will accept any type T and an error,
// handle the error, then return T alone.
func do[T any](t T, err error) T {
	if err != nil {
		panic(err)
	}
	return t
}
