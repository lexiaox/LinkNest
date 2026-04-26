package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"linknest/client/internal/auth"
	clientconfig "linknest/client/internal/config"
	"linknest/client/internal/device"
	"linknest/client/internal/p2p"
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
	case "p2p":
		return runP2P(root, args[1:])
	case "transfer":
		return runTransfer(root, args[1:])
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

	printAuthNotice(result)
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

		printAuthNotice(result)
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

		printAuthNotice(result)
		fmt.Printf("logged in user=%s token_saved=true\n", result.User.Username)
		return nil
	case "delete":
		fs := flag.NewFlagSet("auth delete", flag.ContinueOnError)
		password := fs.String("password", "", "current password")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if strings.TrimSpace(*password) == "" {
			return errors.New("password is required")
		}
		if strings.TrimSpace(cfg.Token) == "" {
			return errors.New("token is empty, run auth login first")
		}

		result, err := client.DeleteAccount(cfg.Token, auth.DeleteAccountInput{
			Password: *password,
		})
		if err != nil {
			return err
		}

		cfg.Token = ""
		if err := clientconfig.Save(root, cfg); err != nil {
			return err
		}

		fmt.Printf("account deleted user=%s token_cleared=true\n", result.User.Username)
		return nil
	default:
		return errors.New("unsupported auth subcommand")
	}
}

func printAuthNotice(result auth.AuthResult) {
	if strings.TrimSpace(result.Notice) == "" {
		return
	}
	fmt.Fprintf(os.Stderr, "notice: %s\n", result.Notice)
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
		items = device.OnlineOnly(items)

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
	case "delete":
		if len(args) < 2 {
			return errors.New("file delete requires a file_id")
		}
		return transfer.DeleteFile(cfg.ServerURL, cfg.Token, args[1])
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

func runP2P(root string, args []string) error {
	if len(args) < 1 {
		return errors.New("p2p command requires a subcommand")
	}

	cfg, err := clientconfig.Load(root)
	if err != nil {
		return err
	}

	switch args[0] {
	case "status":
		fmt.Printf("p2p_enabled=%t\n", clientconfig.P2PEnabledValue(cfg.Transfer))
		fmt.Printf("listen=%s:%d\n", cfg.Transfer.P2PHost, cfg.Transfer.P2PPort)
		fmt.Printf("protocol=http\n")
		fmt.Printf("inbox=%s\n", cfg.Transfer.InboxDir)
		fmt.Printf("virtual_ip=%s\n", strings.TrimSpace(cfg.Transfer.VirtualIP))
		fmt.Printf("fallback_to_cloud=%t\n", clientconfig.FallbackToCloudEnabled(cfg.Transfer))
		return nil
	case "serve":
		fs := flag.NewFlagSet("p2p serve", flag.ContinueOnError)
		host := fs.String("host", cfg.Transfer.P2PHost, "p2p listen host")
		port := fs.Int("port", cfg.Transfer.P2PPort, "p2p listen port")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if !clientconfig.P2PEnabledValue(cfg.Transfer) {
			return errors.New("p2p is disabled in config")
		}
		if strings.TrimSpace(cfg.Token) == "" {
			return errors.New("token is empty, run auth login first")
		}
		profile, err := device.Load(root)
		if err != nil {
			return err
		}
		cfg.Transfer.P2PHost = *host
		cfg.Transfer.P2PPort = *port

		server, err := p2p.Start(root, cfg, profile)
		if err != nil {
			return err
		}
		fmt.Printf("p2p service started addr=%s inbox=%s\n", server.Addr(), cfg.Transfer.InboxDir)

		stopHeartbeat := make(chan struct{})
		go runP2PHeartbeatLoop(cfg, profile, server.Port(), stopHeartbeat)

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
		<-sigCh
		close(stopHeartbeat)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return server.Shutdown(ctx)
	default:
		return errors.New("unsupported p2p subcommand")
	}
}

func runP2PHeartbeatLoop(cfg clientconfig.ClientConfig, profile device.Profile, port int, stop <-chan struct{}) {
	options := device.HeartbeatOptions{
		P2PEnabled:  true,
		P2PPort:     port,
		P2PProtocol: "http",
		VirtualIP:   cfg.Transfer.VirtualIP,
	}
	for {
		err := clientws.RunHeartbeatUntilWithOptions(cfg.ServerURL, cfg.Token, profile, options, 5*time.Second, stop)
		select {
		case <-stop:
			return
		default:
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "p2p heartbeat stopped: %v\n", err)
		}
		time.Sleep(3 * time.Second)
	}
}

func runTransfer(root string, args []string) error {
	if len(args) < 1 {
		return errors.New("transfer command requires a subcommand")
	}

	cfg, err := clientconfig.Load(root)
	if err != nil {
		return err
	}
	if strings.TrimSpace(cfg.Token) == "" {
		return errors.New("token is empty, run auth login first")
	}

	switch args[0] {
	case "send":
		path, targetDeviceID, err := parseTransferSendArgs(args[1:])
		if err != nil {
			return err
		}
		return transfer.Send(root, cfg, path, targetDeviceID)
	case "list":
		items, err := transfer.ListTransfers(cfg.ServerURL, cfg.Token)
		if err != nil {
			return err
		}
		for _, item := range items {
			fmt.Printf("%s\t%s\t%s\t%s\t%s\t%s\n", item.TransferID, item.FileName, item.TargetDeviceID, item.PreferredRoute, item.ActualRoute, item.Status)
		}
		return nil
	case "detail":
		if len(args) < 2 {
			return errors.New("transfer detail requires a transfer_id")
		}
		item, err := transfer.TransferDetail(cfg.ServerURL, cfg.Token, args[1])
		if err != nil {
			return err
		}
		printTransferDetail(item)
		return nil
	case "resume":
		if len(args) < 2 {
			return errors.New("transfer resume requires a transfer_id")
		}
		return transfer.ResumeTransfer(root, cfg, args[1])
	case "fallback":
		if len(args) < 2 {
			return errors.New("transfer fallback requires a transfer_id")
		}
		return transfer.RequestFallback(cfg.ServerURL, cfg.Token, args[1])
	default:
		return errors.New("unsupported transfer subcommand")
	}
}

func parseTransferSendArgs(args []string) (string, string, error) {
	var path string
	var targetDeviceID string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--to":
			if i+1 >= len(args) {
				return "", "", errors.New("--to requires a device_id")
			}
			targetDeviceID = args[i+1]
			i++
		default:
			if path == "" {
				path = args[i]
			}
		}
	}
	if strings.TrimSpace(path) == "" {
		return "", "", errors.New("transfer send requires a file path")
	}
	if strings.TrimSpace(targetDeviceID) == "" {
		return "", "", errors.New("transfer send requires --to <device_id>")
	}
	return path, targetDeviceID, nil
}

func printTransferDetail(item transfer.TransferTask) {
	fmt.Printf("transfer_id=%s\n", item.TransferID)
	fmt.Printf("file=%s size=%d hash=%s\n", item.FileName, item.FileSize, item.FileHash)
	fmt.Printf("source=%s target=%s\n", item.SourceDeviceID, item.TargetDeviceID)
	fmt.Printf("route preferred=%s actual=%s\n", item.PreferredRoute, item.ActualRoute)
	fmt.Printf("chunks=%d chunk_size=%d\n", item.TotalChunks, item.ChunkSize)
	fmt.Printf("status=%s\n", item.Status)
	if strings.TrimSpace(item.SelectedCandidate) != "" {
		fmt.Printf("candidate=%s\n", item.SelectedCandidate)
	}
	if strings.TrimSpace(item.ErrorCode) != "" || strings.TrimSpace(item.ErrorMessage) != "" {
		fmt.Printf("error=%s %s\n", item.ErrorCode, item.ErrorMessage)
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
	fmt.Println("  linknest auth delete --password password")
	fmt.Println("  linknest device init --name demo-pc --type linux")
	fmt.Println("  linknest device register")
	fmt.Println("  linknest device list")
	fmt.Println("  linknest device heartbeat")
	fmt.Println("  linknest file list")
	fmt.Println("  linknest file upload ./demo.zip")
	fmt.Println("  linknest file download <file_id> --output ./downloaded-demo.zip")
	fmt.Println("  linknest file delete <file_id>")
	fmt.Println("  linknest task list")
	fmt.Println("  linknest p2p status")
	fmt.Println("  linknest p2p serve")
	fmt.Println("  linknest transfer send ./demo.zip --to <device_id>")
	fmt.Println("  linknest transfer list")
	fmt.Println("  linknest transfer detail <transfer_id>")
	fmt.Println("  linknest transfer resume <transfer_id>")
	fmt.Println("  linknest transfer fallback <transfer_id>")
	return nil
}
