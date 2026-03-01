package main

import (
	"fmt"
	"io"
	"os"
	"strconv"

	"envsync/internal/envsync"

	"github.com/spf13/cobra"
)

// version is injected at build time:
// go build -ldflags="-X main.version=v1.2.3"
var version = "dev"

type runner interface {
	Init() error
	Login() error
	Logout() error
	WhoAmI() error
	ProjectCreate(name string) error
	ProjectList() error
	ProjectUse(name string) error
	ProjectDelete(name string) error
	TeamCreate(name string) error
	TeamList() error
	TeamUse(name string) error
	TeamAddMember(teamName, actor, role string) error
	TeamRemoveMember(teamName, actor string) error
	TeamListMembers(teamName string) error
	EnvCreate(name string) error
	EnvUse(name string) error
	EnvList() error
	Set(keyName, value, expiresAt string) error
	Rotate(keyName, value string) error
	Get(keyName string) error
	Delete(keyName string) error
	List(showValues bool) error
	Load() error
	ImportEnv(file string) error
	ExportEnv(file string) error
	History(keyName string) error
	Rollback(keyName string, version int) error
	Diff() error
	Push(force bool) error
	Pull(forceRemote bool) error
	PhraseSave() error
	PhraseClear() error
	Doctor() error
	DoctorJSON() error
	Restore() error
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	os.Exit(1)
}

func buildRootCmd(app runner, out io.Writer) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:           "envsync",
		Short:         "encrypted env var sync",
		SilenceUsage:  true,
		SilenceErrors: true,
		Long: "Sync encrypted environment variables across machines.\n\n" +
			"CI example:\n" +
			"  ENVSYNC_RECOVERY_PHRASE='<phrase>' envsync pull --force-remote",
	}
	rootCmd.SetOut(out)

	rootCmd.AddCommand(&cobra.Command{
		Use:   "init",
		Short: "Initialize envsync",
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.Init()
		},
	})
	rootCmd.AddCommand(&cobra.Command{
		Use:   "login",
		Short: "Sign in for cloud sync",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.Login()
		},
	})
	rootCmd.AddCommand(&cobra.Command{
		Use:   "logout",
		Short: "Sign out and clear cloud session",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.Logout()
		},
	})
	rootCmd.AddCommand(&cobra.Command{
		Use:   "whoami",
		Short: "Show cloud account identity",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.WhoAmI()
		},
	})

	projectCmd := &cobra.Command{Use: "project", Short: "Manage projects"}
	rootCmd.AddCommand(projectCmd)
	projectCmd.AddCommand(&cobra.Command{
		Use:   "create <name>",
		Short: "Create a new project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.ProjectCreate(args[0])
		},
	})
	projectCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List projects",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.ProjectList()
		},
	})
	projectCmd.AddCommand(&cobra.Command{
		Use:   "use <name>",
		Short: "Use a project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.ProjectUse(args[0])
		},
	})
	projectCmd.AddCommand(&cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.ProjectDelete(args[0])
		},
	})

	teamCmd := &cobra.Command{Use: "team", Short: "Manage teams"}
	rootCmd.AddCommand(teamCmd)
	teamCmd.AddCommand(&cobra.Command{
		Use:   "create <name>",
		Short: "Create a team",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.TeamCreate(args[0])
		},
	})
	teamCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List teams",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.TeamList()
		},
	})
	teamCmd.AddCommand(&cobra.Command{
		Use:   "use <name>",
		Short: "Use a team",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.TeamUse(args[0])
		},
	})
	teamCmd.AddCommand(&cobra.Command{
		Use:   "add-member <team> <actor> <role>",
		Short: "Add a member to a team",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.TeamAddMember(args[0], args[1], args[2])
		},
	})
	teamCmd.AddCommand(&cobra.Command{
		Use:   "remove-member <team> <actor>",
		Short: "Remove a member from a team",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.TeamRemoveMember(args[0], args[1])
		},
	})
	teamCmd.AddCommand(&cobra.Command{
		Use:   "list-members [team]",
		Short: "List team members",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			teamName := ""
			if len(args) > 0 {
				teamName = args[0]
			}
			return app.TeamListMembers(teamName)
		},
	})

	envCmd := &cobra.Command{Use: "env", Short: "Manage environments"}
	rootCmd.AddCommand(envCmd)
	envCmd.AddCommand(&cobra.Command{
		Use:   "create <name>",
		Short: "Create an environment",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.EnvCreate(args[0])
		},
	})
	envCmd.AddCommand(&cobra.Command{
		Use:   "use <name>",
		Short: "Use an environment",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.EnvUse(args[0])
		},
	})
	envCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List environments",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.EnvList()
		},
	})

	setCmd := &cobra.Command{
		Use:   "set <KEY> <value>",
		Short: "Set a secret",
		Args:  cobra.ExactArgs(2),
		Example: "envsync set API_KEY secret --expires-at 24h\n" +
			"ENVSYNC_RECOVERY_PHRASE='<phrase>' envsync set API_KEY secret",
		RunE: func(cmd *cobra.Command, args []string) error {
			expiresAt, _ := cmd.Flags().GetString("expires-at")
			return app.Set(args[0], args[1], expiresAt)
		},
	}
	setCmd.Flags().String("expires-at", "", "Expiration time (RFC3339 format) or duration (e.g., 24h)")
	rootCmd.AddCommand(setCmd)

	rootCmd.AddCommand(&cobra.Command{
		Use:   "rotate <KEY> <value>",
		Short: "Rotate a secret",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.Rotate(args[0], args[1])
		},
	})
	rootCmd.AddCommand(&cobra.Command{
		Use:   "get <KEY>",
		Short: "Get a secret",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.Get(args[0])
		},
	})
	rootCmd.AddCommand(&cobra.Command{
		Use:   "delete <KEY>",
		Short: "Delete a secret",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.Delete(args[0])
		},
	})

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List secrets in current environment",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			show, _ := cmd.Flags().GetBool("show")
			return app.List(show)
		},
	}
	listCmd.Flags().Bool("show", false, "Show secret values")
	rootCmd.AddCommand(listCmd)

	rootCmd.AddCommand(&cobra.Command{
		Use:   "load",
		Short: "Load secrets into shell exports",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.Load()
		},
	})
	rootCmd.AddCommand(&cobra.Command{
		Use:   "import <file>",
		Short: "Import environment variables from file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.ImportEnv(args[0])
		},
	})
	rootCmd.AddCommand(&cobra.Command{
		Use:   "export <file>",
		Short: "Export environment variables to file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.ExportEnv(args[0])
		},
	})
	rootCmd.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Show version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "envsync %s\n", version)
			return nil
		},
	})

	rootCmd.AddCommand(&cobra.Command{
		Use:   "history <KEY>",
		Short: "Show secret history",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.History(args[0])
		},
	})

	rollbackCmd := &cobra.Command{
		Use:   "rollback <KEY>",
		Short: "Rollback a secret",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			vStr, _ := cmd.Flags().GetString("version")
			if vStr == "" {
				return fmt.Errorf("--version is required")
			}
			v, err := strconv.Atoi(vStr)
			if err != nil {
				return fmt.Errorf("invalid version: %v", err)
			}
			return app.Rollback(args[0], v)
		},
	}
	rollbackCmd.Flags().String("version", "", "Version to rollback to")
	rootCmd.AddCommand(rollbackCmd)

	rootCmd.AddCommand(&cobra.Command{
		Use:   "diff",
		Short: "Show differences between local and remote environments",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.Diff()
		},
	})

	pushCmd := &cobra.Command{
		Use:   "push",
		Short: "Push local changes to remote",
		Args:  cobra.NoArgs,
		Example: "envsync push\n" +
			"ENVSYNC_RECOVERY_PHRASE='<phrase>' envsync push --force",
		RunE: func(cmd *cobra.Command, args []string) error {
			force, _ := cmd.Flags().GetBool("force")
			return app.Push(force)
		},
	}
	pushCmd.Flags().BoolP("force", "f", false, "Force push")
	rootCmd.AddCommand(pushCmd)

	pullCmd := &cobra.Command{
		Use:   "pull",
		Short: "Pull remote changes to local",
		Args:  cobra.NoArgs,
		Example: "envsync pull\n" +
			"ENVSYNC_RECOVERY_PHRASE='<phrase>' envsync pull --force-remote",
		RunE: func(cmd *cobra.Command, args []string) error {
			forceRemote, _ := cmd.Flags().GetBool("force-remote")
			return app.Pull(forceRemote)
		},
	}
	pullCmd.Flags().BoolP("force-remote", "f", false, "Force pull")
	rootCmd.AddCommand(pullCmd)

	phraseCmd := &cobra.Command{Use: "phrase", Short: "Manage recovery phrase"}
	rootCmd.AddCommand(phraseCmd)
	phraseCmd.AddCommand(&cobra.Command{
		Use:   "save",
		Short: "Save phrase to keychain",
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.PhraseSave()
		},
	})
	phraseCmd.AddCommand(&cobra.Command{
		Use:   "clear",
		Short: "Clear phrase from keychain",
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.PhraseClear()
		},
	})

	doctorCmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check system health",
		RunE: func(cmd *cobra.Command, args []string) error {
			asJSON, _ := cmd.Flags().GetBool("json")
			if asJSON {
				return app.DoctorJSON()
			}
			return app.Doctor()
		},
	}
	doctorCmd.Flags().Bool("json", false, "Output checks as JSON for automation")
	rootCmd.AddCommand(doctorCmd)

	rootCmd.AddCommand(&cobra.Command{
		Use:     "restore",
		Short:   "Restore from remote",
		Example: "ENVSYNC_RECOVERY_PHRASE='<phrase>' envsync restore",
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.Restore()
		},
	})

	return rootCmd
}

func main() {
	app, err := envsync.NewApp()
	if err != nil {
		fatal(err)
	}
	if err := buildRootCmd(app, os.Stdout).Execute(); err != nil {
		fatal(err)
	}
}
