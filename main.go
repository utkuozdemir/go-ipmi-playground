package main

import (
	"context"
	"flag"
	"fmt"
	nativeipmi "github.com/bougou/go-ipmi"
	execipmi "github.com/pensando/goipmi"
	"go.uber.org/zap"
	"log"
	"time"
)

type config struct {
	ip       string
	user     string
	pass     string
	port     int
	useLocal bool
}

func main() {
	conf := config{
		ip:       "192.168.178.190",
		user:     "Administrator",
		pass:     "utku1234",
		port:     623,
		useLocal: false,
	}

	flag.StringVar(&conf.ip, "ip", conf.ip, "ip address of the BMC")
	flag.StringVar(&conf.user, "user", conf.user, "username for the BMC")
	flag.StringVar(&conf.pass, "pass", conf.pass, "password for the BMC")
	flag.IntVar(&conf.port, "port", conf.port, "port for the BMC")
	flag.BoolVar(&conf.useLocal, "local", conf.useLocal, "use local client")

	flag.Parse()

	if err := run(conf); err != nil {
		log.Fatalf("failed to run: %v", err)
	}
}

func run(conf config) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	logger, err := zap.NewDevelopment()
	if err != nil {
		return err
	}

	undo := zap.ReplaceGlobals(logger)
	defer undo()

	execClient, nativeClient, err := buildClients(conf)
	if err != nil {
		return fmt.Errorf("error building clients: %w", err)
	}

	if err = execClient.Open(); err != nil {
		return fmt.Errorf("exec: error opening client: %w", err)
	}

	defer func() {
		if closeErr := execClient.Close(); closeErr != nil {
			logger.Error("exec: failed to close session", zap.Error(closeErr))
		}
	}()

	if err = nativeClient.Connect(ctx); err != nil {
		return fmt.Errorf("native: error connecting: %w", err)
	}

	defer func() {
		closeCtx, closeCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer closeCancel()

		if closeErr := nativeClient.Close(closeCtx); closeErr != nil {
			logger.Error("native: failed to close session", zap.Error(closeErr))
		}
	}()

	for range 50 {
		if err = testGetPowerStatus(ctx, execClient, nativeClient, logger); err != nil {
			return fmt.Errorf("failed to test get power status: %w", err)
		}
	}

	//if err = testGetUsername(ctx, execClient, nativeClient, logger); err != nil {
	//	return fmt.Errorf("failed to test get username: %w", err)
	//}
	//
	//if err = testGetUserAccess(ctx, execClient, nativeClient, logger); err != nil {
	//	return fmt.Errorf("failed to test get user access: %w", err)
	//}

	return nil

	//execClient, err := exec.NewClient()
	//if err != nil {
	//	return fmt.Errorf("error creating exec client: %w", err)
	//}
	//
	//// get ip port
	//
	//nativeClient, err := native.NewClient()
	//if err != nil {
	//	return fmt.Errorf("error creating native client: %w", err)
	//}
	//
	//ip, port, err := execClient.GetIPPort()
	//if err != nil {
	//	return fmt.Errorf("error getting bmc ip port: %w", err)
	//}
	//
	//logger.Info("exec client ip port", zap.String("ip", ip), zap.Uint16("port", port))
	//
	//ip, port, err = nativeClient.GetIPPort(context.Background())
	//if err != nil {
	//	return fmt.Errorf("error getting bmc ip port: %w", err)
	//}
	//
	//logger.Info("native client ip port", zap.String("ip", ip), zap.Uint16("port", port))
	//
	//// user exists - true
	//
	//talosAgentUserExists, err := execClient.UserExists("talos-agent")
	//if err != nil {
	//	return fmt.Errorf("error checking if user talos-agent exists: %w", err)
	//}
	//
	//logger.Info("exec talos-agent user exists", zap.Bool("exists", talosAgentUserExists))
	//
	//talosAgentUserExists, err = nativeClient.UserExists(context.Background(), "talos-agent")
	//if err != nil {
	//	return fmt.Errorf("error checking if user talos-agent exists: %w", err)
	//}
	//
	//logger.Info("native talos-agent user exists", zap.Bool("exists", talosAgentUserExists))
	//
	//// user exists - false
	//
	//faikUserExists, err := execClient.UserExists("faik")
	//if err != nil {
	//	return fmt.Errorf("error checking if user faik exists: %w", err)
	//}
	//
	//logger.Info("exec faik user exists", zap.Bool("exists", faikUserExists))
	//
	//faikUserExists, err = nativeClient.UserExists(context.Background(), "faik")
	//if err != nil {
	//	return fmt.Errorf("error checking if user faik exists: %w", err)
	//}
	//
	//logger.Info("native faik user exists", zap.Bool("exists", faikUserExists))
	//
	//return nil
}

func buildClients(conf config) (execClient *execipmi.Client, nativeClient *nativeipmi.Client, err error) {
	if conf.useLocal {
		if execClient, err = execipmi.NewClient(&execipmi.Connection{
			Interface: "open",
		}); err != nil {
			return nil, nil, fmt.Errorf("exec: error creating client: %w", err)
		}

		nativeClient, err = nativeipmi.NewOpenClient()
		if err != nil {
			return nil, nil, fmt.Errorf("native: error creating client: %w", err)
		}

		return execClient, nativeClient, nil
	}

	if execClient, err = execipmi.NewClient(&execipmi.Connection{
		Hostname:  conf.ip,
		Port:      conf.port,
		Username:  conf.user,
		Password:  conf.pass,
		Interface: "lanplus",
	}); err != nil {
		return nil, nil, fmt.Errorf("exec: error creating client: %w", err)
	}

	if nativeClient, err = nativeipmi.NewClient(conf.ip, conf.port, conf.user, conf.pass); err != nil {
		return nil, nil, fmt.Errorf("native: error creating client: %w", err)
	}

	nativeClient = nativeClient.WithDebug(true)

	return execClient, nativeClient, nil
}

func testGetUsername(ctx context.Context, execClient *execipmi.Client, nativeClient *nativeipmi.Client, logger *zap.Logger) error {
	userID := uint8(0x01)

	resp1, err := execClient.GetUserName(userID)
	if err != nil {
		logger.Error("exec: failed to get username", zap.Error(err))
	}

	logger.Info("exec: get username", zap.Any("response", resp1),
		zap.Uint8("completion code", uint8(resp1.CompletionCode)),
		zap.String("username", resp1.Username))

	resp2, err := nativeClient.GetUsername(ctx, userID)
	if err != nil {
		logger.Error("native: failed to get username", zap.Error(err))
	}

	logger.Info("native: get username", zap.Any("response", resp2))

	return nil
}

func testGetUserAccess(ctx context.Context, execClient *execipmi.Client, nativeClient *nativeipmi.Client, logger *zap.Logger) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	channelNumber := uint8(0x01)
	userID := uint8(0x01)

	req1 := &execipmi.Request{
		NetworkFunction: execipmi.NetworkFunctionApp,
		Command:         execipmi.CommandGetUserSummary,
		Data: &execipmi.GetUserSummaryRequest{
			ChannelNumber: channelNumber,
			UserID:        userID,
		},
	}

	resp1 := &execipmi.GetUserSummaryResponse{}

	err := execClient.Send(req1, resp1)
	if err != nil {
		return fmt.Errorf("exec: failed to get user summary: %w", err)
	}

	execMaxUsers := resp1.MaxUsers & 0x1F // Only bits [0:5] provide this number
	execCurrUsers := resp1.CurrEnabledUsers & 0x1F

	logger.Info("exec: get user access",
		zap.Any("response", resp1),
		zap.Uint8("max users", execMaxUsers),
		zap.Uint8("current users", execCurrUsers))

	resp2, err := nativeClient.GetUserAccess(ctx, channelNumber, userID)
	if err != nil {
		logger.Error("native: failed to get user access", zap.Error(err))
	}

	logger.Info("native: get user access", zap.Any("response", resp2),
		zap.Uint8("max users", resp2.MaxUsersIDCount),
		zap.Uint8("enabled users", resp2.EnabledUserIDsCount))

	return nil
}

func testGetPowerStatus(ctx context.Context, execClient *execipmi.Client, nativeClient *nativeipmi.Client, logger *zap.Logger) error {
	resp, err := nativeClient.GetChassisStatus(ctx)
	if err != nil {
		logger.Error("native: failed to get chassis status", zap.Error(err))
		return err
	}

	logger.Info("native: get chassis status", zap.Bool("powered_on", resp.PowerIsOn))

	req := &execipmi.Request{
		NetworkFunction: execipmi.NetworkFunctionChassis,
		Command:         execipmi.CommandChassisStatus,
		Data:            execipmi.ChassisStatusRequest{},
	}

	res := &execipmi.ChassisStatusResponse{}

	err = execClient.Send(req, res)
	if err != nil {
		logger.Error("exec: failed to get power status", zap.Error(err))
		return err
	}

	logger.Info("exec: get power status", zap.Bool("powered_on", res.IsSystemPowerOn()))

	return nil

}
