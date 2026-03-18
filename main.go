// cloudshell-term — interactive AWS CloudShell terminal from your CLI.
//
// Usage:
//
//	cloudshell-term [region]
//
// Connects to an AWS CloudShell environment in the specified region
// (defaults to AWS_DEFAULT_REGION or us-east-1) and gives you an
// interactive terminal session. No AWS Console required.
//
// Requires: AWS credentials and session-manager-plugin installed.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/google/uuid"
)

// ---- CloudShell API client ----

type csClient struct {
	httpClient *http.Client
	signer     *v4.Signer
	region     string
	endpoint   string
	creds      aws.CredentialsProvider
}

type environment struct {
	EnvironmentID string `json:"EnvironmentId"`
	Status        string `json:"Status"`
}

type describeOutput struct {
	Environments []environment `json:"Environments"`
}

type sessionOutput struct {
	SessionID  string `json:"SessionId"`
	TokenValue string `json:"TokenValue"`
	StreamURL  string `json:"StreamUrl"`
}

func newCSClient(cfg aws.Config, region string) *csClient {
	return &csClient{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		signer:     v4.NewSigner(),
		region:     region,
		endpoint:   fmt.Sprintf("https://cloudshell.%s.amazonaws.com", region),
		creds:      cfg.Credentials,
	}
}

func (c *csClient) do(ctx context.Context, target string, payload any, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/%s", c.endpoint, target), bytes.NewReader(body))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Amz-Target", fmt.Sprintf("AWSCloudShell.%s", capitalize(target)))

	creds, err := c.creds.Retrieve(ctx)
	if err != nil {
		return fmt.Errorf("retrieve credentials: %w", err)
	}

	sum := sha256.Sum256(body)
	if err := c.signer.SignHTTP(ctx, creds, req, hex.EncodeToString(sum[:]),
		"cloudshell", c.region, time.Now()); err != nil {
		return fmt.Errorf("sign: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("api error (http %d): %s", resp.StatusCode, string(raw))
	}

	if out != nil {
		return json.Unmarshal(raw, out)
	}
	return nil
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func (c *csClient) describeEnvironments(ctx context.Context) (*describeOutput, error) {
	var out describeOutput
	err := c.do(ctx, "describeEnvironments", struct{}{}, &out)
	return &out, err
}

func (c *csClient) createEnvironment(ctx context.Context) (*environment, error) {
	var out environment
	err := c.do(ctx, "createEnvironment", struct{}{}, &out)
	return &out, err
}

func (c *csClient) getEnvironmentStatus(ctx context.Context, envID string) (string, error) {
	var out struct {
		Status string `json:"Status"`
	}
	err := c.do(ctx, "getEnvironmentStatus", map[string]string{"EnvironmentId": envID}, &out)
	return out.Status, err
}

func (c *csClient) startEnvironment(ctx context.Context, envID string) error {
	return c.do(ctx, "startEnvironment", map[string]string{"EnvironmentId": envID}, nil)
}

func (c *csClient) createSession(ctx context.Context, envID string) (*sessionOutput, error) {
	var out sessionOutput
	err := c.do(ctx, "createSession", map[string]any{
		"EnvironmentId": envID,
		"SessionType":   "TMUX",
		"TabId":         uuid.NewString(),
		"QCliDisabled":  true,
	}, &out)
	return &out, err
}

func (c *csClient) deleteSession(ctx context.Context, envID, sessID string) error {
	return c.do(ctx, "deleteSession", map[string]string{
		"EnvironmentId": envID,
		"SessionId":     sessID,
	}, nil)
}

func (c *csClient) sendHeartbeat(ctx context.Context, envID string) error {
	return c.do(ctx, "sendHeartBeat", map[string]string{"EnvironmentId": envID}, nil)
}

// ---- Main ----

func main() {
	region := os.Getenv("AWS_DEFAULT_REGION")
	if region == "" {
		region = os.Getenv("AWS_REGION")
	}
	if region == "" {
		region = "us-east-1"
	}

	if len(os.Args) > 1 {
		arg := os.Args[1]
		if arg == "--help" || arg == "-h" {
			fmt.Println("Usage: cloudshell-term [region]")
			fmt.Println()
			fmt.Println("Opens an interactive AWS CloudShell terminal.")
			fmt.Println("Defaults to AWS_DEFAULT_REGION or us-east-1.")
			fmt.Println()
			fmt.Println("Requires: AWS credentials and session-manager-plugin")
			os.Exit(0)
		}
		region = arg
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, region); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, region string) error {
	fmt.Fprintf(os.Stderr, "Connecting to CloudShell in %s...\n", region)

	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return fmt.Errorf("load AWS config: %w", err)
	}

	client := newCSClient(cfg, region)

	// Get or create environment
	envs, err := client.describeEnvironments(ctx)
	if err != nil {
		return fmt.Errorf("describe environments: %w", err)
	}

	var env environment
	if len(envs.Environments) == 0 {
		fmt.Fprintf(os.Stderr, "No environment found, creating...\n")
		created, err := client.createEnvironment(ctx)
		if err != nil {
			return fmt.Errorf("create environment: %w", err)
		}
		env = *created
	} else {
		env = envs.Environments[0]
	}

	// Check status and start if needed
	status := env.Status
	if status == "" {
		s, _ := client.getEnvironmentStatus(ctx, env.EnvironmentID)
		if s != "" {
			status = s
		}
	}

	if status != "RUNNING" {
		if status == "SUSPENDED" {
			fmt.Fprintf(os.Stderr, "Starting environment...\n")
			if err := client.startEnvironment(ctx, env.EnvironmentID); err != nil {
				return fmt.Errorf("start: %w", err)
			}
		} else {
			fmt.Fprintf(os.Stderr, "Environment status: %s, waiting...\n", status)
		}

		for i := 0; i < 90; i++ {
			time.Sleep(2 * time.Second)
			s, _ := client.getEnvironmentStatus(ctx, env.EnvironmentID)
			if s == "RUNNING" {
				break
			}
			if s == "SUSPENDED" {
				client.startEnvironment(ctx, env.EnvironmentID)
			}
			if i > 0 && i%5 == 0 {
				fmt.Fprintf(os.Stderr, "Still waiting (%s)...\n", s)
			}
		}
	}

	// Create session
	sess, err := client.createSession(ctx, env.EnvironmentID)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	defer client.deleteSession(context.Background(), env.EnvironmentID, sess.SessionID)

	fmt.Fprintf(os.Stderr, "Connected.\n\n")

	// Start heartbeat in background
	go func() {
		ticker := time.NewTicker(4 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				client.sendHeartbeat(ctx, env.EnvironmentID)
			}
		}
	}()

	// Launch session-manager-plugin with stdin/stdout/stderr connected directly
	payload, _ := json.Marshal(map[string]string{
		"SessionId":  sess.SessionID,
		"TokenValue": sess.TokenValue,
		"StreamUrl":  sess.StreamURL,
	})

	cmd := exec.CommandContext(ctx, "session-manager-plugin", string(payload), region, "StartSession")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		// Don't report error if user hit Ctrl+C
		if ctx.Err() != nil {
			return nil
		}
		return fmt.Errorf("session-manager-plugin: %w", err)
	}

	return nil
}
