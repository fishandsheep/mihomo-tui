package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/metacubex/mihomo-tui/internal/app"
	"github.com/metacubex/mihomo-tui/internal/profile"
)

var version = "dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return openCommand(nil)
	}

	switch args[0] {
	case "open":
		return openCommand(args[1:])
	case "profile":
		return profileCommand(args[1:])
	case "-v", "--version", "version":
		printVersion()
		return nil
	case "-h", "--help", "help":
		printUsage()
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func printUsage() {
	fmt.Println(`Usage:
  mihomo-tui open --profile <name>
  mihomo-tui open --controller http://127.0.0.1:9090 --secret xxx
  mihomo-tui profile add --name <name> --controller <url> [--secret xxx] [--tls-skip-verify] [--default]
  mihomo-tui profile edit --name <name> [--controller <url>] [--secret xxx] [--tls-skip-verify=true|false] [--default=true|false]
  mihomo-tui profile remove --name <name>
  mihomo-tui profile list
  mihomo-tui version`)
}

func printVersion() {
	fmt.Printf("mihomo-tui %s\n", version)
}

func openCommand(args []string) error {
	fs := flag.NewFlagSet("open", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	profileName := fs.String("profile", "", "profile name")
	controllerURL := fs.String("controller", "", "controller URL")
	secret := fs.String("secret", "", "controller secret")
	tlsSkipVerify := fs.Bool("tls-skip-verify", false, "skip TLS verification")
	if err := fs.Parse(args); err != nil {
		return err
	}

	store, err := profile.NewStore("")
	if err != nil {
		return err
	}

	var selected profile.Profile
	switch {
	case *profileName != "":
		p, ok := store.Get(*profileName)
		if !ok {
			return fmt.Errorf("profile %q not found", *profileName)
		}
		selected = p
	case *controllerURL != "":
		selected = profile.Profile{
			Name:          "direct",
			ControllerURL: *controllerURL,
			Secret:        *secret,
			TLSSkipVerify: *tlsSkipVerify,
		}
	default:
		p, ok := store.Default()
		if !ok {
			return errors.New("no profile selected; pass --profile or --controller")
		}
		selected = p
	}

	model := app.NewModel(app.Options{
		Store:          store,
		InitialProfile: selected.Name,
		DirectProfile:  selected,
	})

	_, err = tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion()).Run()
	return err
}

func profileCommand(args []string) error {
	if len(args) == 0 {
		return errors.New("missing profile subcommand")
	}

	store, err := profile.NewStore("")
	if err != nil {
		return err
	}

	switch args[0] {
	case "add":
		return profileAdd(store, args[1:])
	case "edit":
		return profileEdit(store, args[1:])
	case "remove":
		return profileRemove(store, args[1:])
	case "list":
		return profileList(store)
	default:
		return fmt.Errorf("unknown profile subcommand %q", args[0])
	}
}

func profileAdd(store *profile.Store, args []string) error {
	fs := flag.NewFlagSet("profile add", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	name := fs.String("name", "", "profile name")
	controllerURL := fs.String("controller", "", "controller URL")
	secret := fs.String("secret", "", "controller secret")
	tlsSkipVerify := fs.Bool("tls-skip-verify", false, "skip TLS verification")
	setDefault := fs.Bool("default", false, "set default profile")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *name == "" || *controllerURL == "" {
		return errors.New("--name and --controller required")
	}

	return store.Upsert(profile.Profile{
		Name:          *name,
		ControllerURL: *controllerURL,
		Secret:        *secret,
		TLSSkipVerify: *tlsSkipVerify,
		Default:       *setDefault,
	})
}

func profileEdit(store *profile.Store, args []string) error {
	fs := flag.NewFlagSet("profile edit", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	name := fs.String("name", "", "profile name")
	controllerURL := fs.String("controller", "", "controller URL")
	secret := fs.String("secret", "", "controller secret")
	tlsSkipVerify := fs.String("tls-skip-verify", "", "skip TLS verification")
	defaultFlag := fs.String("default", "", "set default profile")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *name == "" {
		return errors.New("--name required")
	}

	current, ok := store.Get(*name)
	if !ok {
		return fmt.Errorf("profile %q not found", *name)
	}

	if *controllerURL != "" {
		current.ControllerURL = *controllerURL
	}
	if *secret != "" || hasFlag(args, "--secret") {
		current.Secret = *secret
	}
	if *tlsSkipVerify != "" {
		switch strings.ToLower(*tlsSkipVerify) {
		case "true":
			current.TLSSkipVerify = true
		case "false":
			current.TLSSkipVerify = false
		default:
			return errors.New("--tls-skip-verify must be true or false")
		}
	}
	if *defaultFlag != "" {
		switch strings.ToLower(*defaultFlag) {
		case "true":
			current.Default = true
		case "false":
			current.Default = false
		default:
			return errors.New("--default must be true or false")
		}
	}

	return store.Upsert(current)
}

func profileRemove(store *profile.Store, args []string) error {
	fs := flag.NewFlagSet("profile remove", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	name := fs.String("name", "", "profile name")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *name == "" {
		return errors.New("--name required")
	}
	return store.Remove(*name)
}

func profileList(store *profile.Store) error {
	for _, item := range store.List() {
		line := item.Name + "\t" + item.ControllerURL
		if item.Default {
			line += "\tdefault"
		}
		fmt.Println(line)
	}
	return nil
}

func hasFlag(args []string, name string) bool {
	for _, arg := range args {
		if arg == name || strings.HasPrefix(arg, name+"=") {
			return true
		}
	}
	return false
}
