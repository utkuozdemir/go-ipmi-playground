package native

import (
	"context"
	"errors"
	"fmt"
	"github.com/bougou/go-ipmi"
	"go.uber.org/zap"
	"strings"
	"time"
)

const channelNumber = uint8(0x01)

// Client is a holder for the ipmiClient.
type Client struct {
	ipmiClient *ipmi.Client
}

// NewClient creates a new local ipmi client to use.
func NewClient() (*Client, error) {
	ipmiClient, err := ipmi.NewOpenClient()
	if err != nil {
		return nil, err
	}

	// Local client does not do anything with the context,
	// so we can just use the background context.
	// Still, to be on the safe side, we will use a context with a timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err = ipmiClient.Connect(ctx); err != nil {
		return nil, err
	}

	return &Client{ipmiClient: ipmiClient}, nil
}

// Close the client.
func (c *Client) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return c.ipmiClient.Close(ctx)
}

func (c *Client) AttemptUserSetup(ctx context.Context, username, password string, logger *zap.Logger) error {
	userAccessResponse, err := c.ipmiClient.GetUserAccess(ctx, channelNumber, 0x01)
	if err != nil {
		return err
	}

	userExists := false
	userID := uint8(0)

	// Start from user ID 2, because 1 is the default admin user
	for i := uint8(2); i <= userAccessResponse.MaxUsersIDCount; i++ {
		userRes, userErr := c.ipmiClient.GetUsername(ctx, i)
		if userErr != nil { // This is an empty slot
			if userID == 0 { // We haven't found an empty slot yet
				userID = i // Note this empty slot
			}

			continue
		}

		if userRes.Username == username { // User already exists
			userExists = true
			userID = i

			logger.Info("user already present in slot, we'll claim it as our own", zap.Uint8("slot", i))

			break
		}

		isEmptySlot := userRes.Username == "" || strings.TrimSpace(userRes.Username) == "(Empty User)"

		if isEmptySlot && userID == 0 { // This slot is empty, and we haven't found an empty slot yet
			userID = i // Note this empty slot
		}
	}

	if userID == 0 { // No existing user found, and no empty slot available
		return errors.New("no slot available for user")
	}

	if !userExists { // Add our user to the empty slot
		logger.Info("adding user to slot", zap.Uint8("slot", userID))

		if _, err = c.ipmiClient.SetUsername(ctx, userID, username); err != nil {
			return err
		}
	}

	if _, err = c.ipmiClient.SetUserPassword(ctx, userID, password, false); err != nil {
		return err
	}

	if _, err = c.ipmiClient.SetUserAccess(ctx, &ipmi.SetUserAccessRequest{
		EnableChanging:      true,
		EnableIPMIMessaging: true,
		ChannelNumber:       uint8(ipmi.ChannelMediumIPMB),
		UserID:              userID,
		MaxPrivLevel:        uint8(ipmi.PrivilegeLevelAdministrator),
	}); err != nil {
		return fmt.Errorf("failed to set user access: %w", err)
	}

	if err = c.ipmiClient.EnableUser(ctx, userID); err != nil {
		return fmt.Errorf("failed to enable user: %w", err)
	}

	return nil
}

// UserExists checks if a user exists on the BMC.
func (c *Client) UserExists(ctx context.Context, username string) (bool, error) {
	users, err := c.ipmiClient.ListUser(ctx, channelNumber)
	if err != nil {
		return false, err
	}

	for _, user := range users {
		if user.Name == username {
			return true, nil
		}
	}

	return false, nil
}

// GetIPPort returns the IPMI IP and port.
func (c *Client) GetIPPort(ctx context.Context) (string, uint16, error) {
	var (
		ipParam   ipmi.LanConfigParam_IP
		portParam ipmi.LanConfigParam_PrimaryRMCPPort
	)

	if err := c.ipmiClient.GetLanConfigParamFor(ctx, channelNumber, &ipParam); err != nil {
		return "", 0, err
	}

	if err := c.ipmiClient.GetLanConfigParamFor(ctx, channelNumber, &portParam); err != nil {
		return "", 0, err
	}

	return ipParam.IP.String(), portParam.Port, nil
}
