package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"

	"envsync/internal/envsync"
)

func main() {
	app, err := envsync.NewApp()
	if err != nil {
		fatal(err)
	}
	if err := run(app, os.Args[1:]); err != nil {
		fatal(err)
	}
}

func run(app *envsync.App, args []string) error {
	if len(args) == 0 {
		printHelp()
		return nil
	}
	switch args[0] {
	case "init":
		return app.Init()
	case "project":
		if len(args) < 2 {
			return errors.New("usage: envsync project <create|list|use> [name]")
		}
		switch args[1] {
		case "create":
			if len(args) < 3 {
				return errors.New("usage: envsync project create <name>")
			}
			return app.ProjectCreate(args[2])
		case "list":
			return app.ProjectList()
		case "use":
			if len(args) < 3 {
				return errors.New("usage: envsync project use <name>")
			}
			return app.ProjectUse(args[2])
		default:
			return errors.New("usage: envsync project <create|list|use> [name]")
		}
	case "env":
		if len(args) < 2 {
			return errors.New("usage: envsync env <create|use> [name]")
		}
		switch args[1] {
		case "create":
			if len(args) < 3 {
				return errors.New("usage: envsync env create <name>")
			}
			return app.EnvCreate(args[2])
		case "use":
			if len(args) < 3 {
				return errors.New("usage: envsync env use <name>")
			}
			return app.EnvUse(args[2])
		default:
			return errors.New("usage: envsync env <create|use> [name]")
		}
	case "team":
		if len(args) < 2 {
			return errors.New("usage: envsync team <create|list|use|add-member|list-members> ...")
		}
		switch args[1] {
		case "create":
			if len(args) < 3 {
				return errors.New("usage: envsync team create <name>")
			}
			return app.TeamCreate(args[2])
		case "list":
			return app.TeamList()
		case "use":
			if len(args) < 3 {
				return errors.New("usage: envsync team use <name>")
			}
			return app.TeamUse(args[2])
		case "add-member":
			if len(args) < 5 {
				return errors.New("usage: envsync team add-member <team> <actor> <role>")
			}
			return app.TeamAddMember(args[2], args[3], args[4])
		case "list-members":
			if len(args) > 2 {
				return app.TeamListMembers(args[2])
			}
			return app.TeamListMembers("")
		default:
			return errors.New("usage: envsync team <create|list|use|add-member|list-members> ...")
		}
	case "set":
		if len(args) < 3 {
			return errors.New("usage: envsync set <KEY> <value>")
		}
		return app.Set(args[1], args[2])
	case "rotate":
		if len(args) < 3 {
			return errors.New("usage: envsync rotate <KEY> <value>")
		}
		return app.Rotate(args[1], args[2])
	case "get":
		if len(args) < 2 {
			return errors.New("usage: envsync get <KEY>")
		}
		return app.Get(args[1])
	case "delete":
		if len(args) < 2 {
			return errors.New("usage: envsync delete <KEY>")
		}
		return app.Delete(args[1])
	case "list":
		show := len(args) > 1 && args[1] == "--show"
		return app.List(show)
	case "load":
		return app.Load()
	case "history":
		if len(args) < 2 {
			return errors.New("usage: envsync history <KEY>")
		}
		return app.History(args[1])
	case "rollback":
		if len(args) < 4 || args[2] != "--version" {
			return errors.New("usage: envsync rollback <KEY> --version <n>")
		}
		v, err := strconv.Atoi(args[3])
		if err != nil {
			return fmt.Errorf("invalid version %q", args[3])
		}
		return app.Rollback(args[1], v)
	case "push":
		force := len(args) > 1 && args[1] == "--force"
		return app.Push(force)
	case "pull":
		forceRemote := len(args) > 1 && args[1] == "--force-remote"
		return app.Pull(forceRemote)
	case "phrase":
		if len(args) < 2 {
			return errors.New("usage: envsync phrase <save|clear>")
		}
		switch args[1] {
		case "save":
			return app.PhraseSave()
		case "clear":
			return app.PhraseClear()
		default:
			return errors.New("usage: envsync phrase <save|clear>")
		}
	case "doctor":
		return app.Doctor()
	case "restore":
		return app.Restore()
	case "help", "-h", "--help":
		printHelp()
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func printHelp() {
	fmt.Print(`envsync - encrypted env var sync

Usage:
  envsync init
  envsync project create <name>
  envsync project list
  envsync project use <name>
  envsync team create <name>
  envsync team list
  envsync team use <name>
  envsync team add-member <team> <actor> <role>
  envsync team list-members [team]
  envsync env create <name>
  envsync env use <name>
  envsync set <KEY> <value>
  envsync rotate <KEY> <value>
  envsync get <KEY>
  envsync delete <KEY>
  envsync list [--show]
  envsync load
  envsync history <KEY>
  envsync rollback <KEY> --version <n>
  envsync push [--force]
  envsync pull [--force-remote]
  envsync phrase save
  envsync phrase clear
  envsync doctor
  envsync restore
`)
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	os.Exit(1)
}
