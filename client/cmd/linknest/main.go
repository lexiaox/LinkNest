package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"linknest/client/internal/auth"
	clientconfig "linknest/client/internal/config"
	"linknest/client/internal/device"
	"linknest/client/internal/transfer"
	clientws "linknest/client/internal/websocket"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) < 1 {
		return usage()
	}

	root, err := clientconfig.RootDir()
	if err != nil {
		return err
	}
	if err := clientconfig.EnsureRoot(root); err != nil {
		return err
	}

	switch args[0] {
	case "setup":
		return runSetup(root, args[1:])
	case "register":
		return runRegisterShortcut(root, args[1:])
	case "login":
		return runLoginShortcut(root, args[1:])
	case "online":
		return runDevice(root, []string{"heartbeat"})
	case "auth":
		return runAuth(root, args[1:])
	case "device":
		return runDevice(root, args[1:])
	case "file":
		return runFile(root, args[1:])
	case "task":
		return runTask(root, args[1:])
	default:
		return usage()
	}
}

func runSetup(root string, args []string) error {
	fs := flag.NewFlagSet("setup", flag.ContinueOnError)
	register := fs.Bool("register", false, "create a new account before binding the device")
	username := fs.String("username", "", "username")
	email := fs.String("email", "", "email, required with --register")
	password := fs.String("password", "", "password")
	name := fs.String("name", "", "device name, defaults to hostname")
	deviceType := fs.String("type", "", "device type, defaults to current OS")
	version := fs.String("version", "0.1.0", "client version")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if strings.TrimSpace(*username) == "" {
		return errors.New("username is required")
	}
	if strings.TrimSpace(*password) == "" {
		return errors.New("password is required")
	}
	if *register && strings.TrimSpace(*email) == "" {
		return errors.New("email is required when --register is set")
	}

	cfg, err := clientconfig.Load(root)
	if err != nil {
		return err
	}

	var result auth.AuthResult
	if *register {
		result, err = registerUser(cfg.ServerURL, auth.RegisterInput{
			Username: *username,
			Email:    *email,
			Password: *password,
		})
	} else {
		result, err = loginUser(cfg.ServerURL, auth.LoginInput{
			Username: *username,
			Password: *password,
		})
	}
	if err != nil {
		return err
	}

	cfg.Token = result.Token
	profile, err := device.Init(root, *name, *deviceType, *version)
	if err != nil {
		return err
	}

	cfg.Device = clientconfig.DeviceConfig{
		DeviceID:      profile.DeviceID,
		DeviceName:    profile.DeviceName,
		DeviceType:    profile.DeviceType,
		ClientVersion: profile.ClientVersion,
	}
	if err := clientconfig.Save(root, cfg); err != nil {
		return err
	}

	if err := device.Register(cfg.ServerURL, cfg.Token, profile); err != nil {
		return err
	}

	fmt.Printf("setup complete user=%s device_id=%s device_name=%s device_type=%s\n", result.User.Username, profile.DeviceID, profile.DeviceName, profile.DeviceType)
	fmt.Println("run `linknest online` to keep this device marked online")
	return nil
}

func runRegisterShortcut(root string, args []string) error {
	return runAuth(root, append([]string{"register"}, args...))
}

func runLoginShortcut(root string, args []string) error {
	return runAuth(root, append([]string{"login"}, args...))
}

func runAuth(root string, args []string) error {
	if len(args) < 1 {
		return errors.New("auth command requires a subcommand")
	}

	cfg, err := clientconfig.Load(root)
	if err != nil {
		return err
	}
	client := auth.NewClient(cfg.ServerURL)

	switch args[0] {
	case "register":
		fs := flag.NewFlagSet("auth register", flag.ContinueOnError)
		username := fs.String("username", "", "username")
		email := fs.String("email", "", "email")
		password := fs.String("password", "", "password")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}

		result, err := client.Register(auth.RegisterInput{
			Username: *username,
			Email:    *email,
			Password: *password,
		})
		if err != nil {
			return err
		}

		cfg.Token = result.Token
		if err := clientconfig.Save(root, cfg); err != nil {
			return err
		}

		fmt.Printf("registered user=%s token_saved=true\n", result.User.Username)
		return nil
	case "login":
		fs := flag.NewFlagSet("auth login", flag.ContinueOnError)
		username := fs.String("username", "", "username")
		password := fs.String("password", "", "password")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}

		result, err := client.Login(auth.LoginInput{
			Username: *username,
			Password: *password,
		})
		if err != nil {
			return err
		}

		cfg.Token = result.Token
		if err := clientconfig.Save(root, cfg); err != nil {
			return err
		}

		fmt.Printf("logged in user=%s token_saved=true\n", result.User.Username)
		return nil
	default:
		return errors.New("unsupported auth subcommand")
	}
}

func registerUser(serverURL string, input auth.RegisterInput) (auth.AuthResult, error) {
	return auth.NewClient(serverURL).Register(input)
}

func loginUser(serverURL string, input auth.LoginInput) (auth.AuthResult, error) {
	return auth.NewClient(serverURL).Login(input)
}

func runDevice(root string, args []string) error {
	if len(args) < 1 {
		return errors.New("device command requires a subcommand")
	}

	cfg, err := clientconfig.Load(root)
	if err != nil {
		return err
	}

	switch args[0] {
	case "init":
		fs := flag.NewFlagSet("device init", flag.ContinueOnError)
		name := fs.String("name", "", "device name")
		deviceType := fs.String("type", "", "device type")
		version := fs.String("version", "0.1.0", "client version")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}

		profile, err := device.Init(root, *name, *deviceType, *version)
		if err != nil {
			return err
		}

		cfg.Device = clientconfig.DeviceConfig{
			DeviceID:      profile.DeviceID,
			DeviceName:    profile.DeviceName,
			DeviceType:    profile.DeviceType,
			ClientVersion: profile.ClientVersion,
		}
		if err := clientconfig.Save(root, cfg); err != nil {
			return err
		}

		fmt.Printf("device initialized device_id=%s\n", profile.DeviceID)
		return nil
	case "register":
		profile, err := device.Load(root)
		if err != nil {
			return err
		}
		if strings.TrimSpace(cfg.Token) == "" {
			return errors.New("token is empty, run auth login first")
		}

		if err := device.Register(cfg.ServerURL, cfg.Token, profile); err != nil {
			return err
		}
		fmt.Printf("device registered device_id=%s\n", profile.DeviceID)
		return nil
	case "list":
		if strings.TrimSpace(cfg.Token) == "" {
			return errors.New("token is empty, run auth login first")
		}

		items, err := device.List(cfg.ServerURL, cfg.Token)
		if err != nil {
			return err
		}

		for _, item := range items {
			fmt.Printf("%s\t%s\t%s\t%s\t%s\n", item.DeviceID, item.DeviceName, item.DeviceType, item.Status, item.LastSeenAt)
		}
		return nil
	case "heartbeat":
		profile, err := device.Load(root)
		if err != nil {
			return err
		}
		if strings.TrimSpace(cfg.Token) == "" {
			return errors.New("token is empty, run auth login first")
		}

		for {
			err := clientws.RunHeartbeat(cfg.ServerURL, cfg.Token, profile, 5*time.Second)
			if err != nil {
				fmt.Fprintf(os.Stderr, "heartbeat stopped: %v\n", err)
			}
			time.Sleep(3 * time.Second)
		}
	default:
		return errors.New("unsupported device subcommand")
	}
}

func runFile(root string, args []string) error {
	if len(args) < 1 {
		return errors.New("file command requires a subcommand")
	}

	cfg, err := clientconfig.Load(root)
	if err != nil {
		return err
	}
	if strings.TrimSpace(cfg.Token) == "" {
		return errors.New("token is empty, run auth login first")
	}

	switch args[0] {
	case "list":
		items, err := transfer.ListFiles(cfg.ServerURL, cfg.Token)
		if err != nil {
			return err
		}
		for _, item := range items {
			fmt.Printf("%s\t%s\t%d\t%s\n", item.FileID, item.FileName, item.FileSize, item.Status)
		}
		return nil
	case "upload":
		if len(args) < 2 {
			return errors.New("file upload requires a file path")
		}
		return transfer.Upload(root, cfg, args[1])
	case "download":
		if len(args) < 2 {
			return errors.New("file download requires a file_id")
		}
		output := ""
		if len(args) > 3 && args[2] == "--output" {
			output = args[3]
		}
		return transfer.Download(root, cfg, args[1], output)
	default:
		return errors.New("unsupported file subcommand")
	}
}

func runTask(root string, args []string) error {
	if len(args) < 1 {
		return errors.New("task command requires a subcommand")
	}

	cfg, err := clientconfig.Load(root)
	if err != nil {
		return err
	}
	if strings.TrimSpace(cfg.Token) == "" {
		return errors.New("token is empty, run auth login first")
	}

	switch args[0] {
	case "list":
		items, err := transfer.ListTasks(cfg.ServerURL, cfg.Token)
		if err != nil {
			return err
		}
		for _, item := range items {
			fmt.Printf("%s\t%s\t%d/%d\t%s\n", item.UploadID, item.FileName, item.UploadedChunks, item.TotalChunks, item.Status)
		}
		return nil
	case "resume":
		if len(args) < 2 {
			return errors.New("task resume requires an upload_id")
		}
		return transfer.Resume(root, cfg, args[1])
	default:
		return errors.New("unsupported task subcommand")
	}
}

func usage() error {
	fmt.Println("LinkNest CLI")
	fmt.Println("usage:")
	fmt.Println("  linknest setup --username demo --password password")
	fmt.Println("  linknest setup --register --username demo --email demo@example.com --password password")
	fmt.Println("  linknest login --username demo --password password")
	fmt.Println("  linknest register --username demo --email demo@example.com --password password")
	fmt.Println("  linknest online")
	fmt.Println("  linknest auth register --username demo --email demo@example.com --password password")
	fmt.Println("  linknest auth login --username demo --password password")
	fmt.Println("  linknest device init --name demo-pc --type linux")
	fmt.Println("  linknest device register")
	fmt.Println("  linknest device list")
	fmt.Println("  linknest device heartbeat")
	fmt.Println("  linknest file list")
	fmt.Println("  linknest task list")
	return nil
}
