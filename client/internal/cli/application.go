package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/GreenFuze/MyGamesAnywhere/client/internal/buildinfo"
	"github.com/GreenFuze/MyGamesAnywhere/client/internal/clientapp"
	clientconfig "github.com/GreenFuze/MyGamesAnywhere/client/internal/config"
	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
	"github.com/spf13/cobra"
)

// Dependencies contains the process-level services required by the CLI.
type Dependencies struct {
	Out       io.Writer
	Err       io.Writer
	BuildInfo buildinfo.Info
	Client    ClientService
}

type ClientService interface {
	Pair(ctx context.Context, options clientapp.PairOptions) (clientconfig.Config, error)
	Start(ctx context.Context, options clientapp.StartOptions) error
	RunAgent(ctx context.Context) error
	Status() (clientapp.Status, error)
	Doctor(ctx context.Context) (clientapp.DoctorResult, error)
	Unpair() error
}

// Application owns the CLI command graph and its injected dependencies.
type Application struct {
	root *cobra.Command
}

// NewApplication constructs a complete command graph or fails immediately when
// required process dependencies are missing.
func NewApplication(deps Dependencies) (*Application, error) {
	if deps.Out == nil {
		return nil, errors.New("output writer is required")
	}
	if deps.Err == nil {
		return nil, errors.New("error writer is required")
	}
	if strings.TrimSpace(deps.BuildInfo.Version) == "" {
		return nil, errors.New("build version is required")
	}
	if deps.Client == nil {
		return nil, errors.New("client service is required")
	}

	root := &cobra.Command{
		Use:           "mga-client",
		Short:         "Per-user device agent and CLI for MyGamesAnywhere",
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	root.SetOut(deps.Out)
	root.SetErr(deps.Err)
	root.AddCommand(newVersionCommand(deps.BuildInfo))
	root.AddCommand(newPairCommand(deps.Client))
	root.AddCommand(newAgentCommand(deps.Client))
	root.AddCommand(newStatusCommand(deps.Client))
	root.AddCommand(newDoctorCommand(deps.Client))
	root.AddCommand(newUnpairCommand(deps.Client))
	root.AddCommand(newProtocolCommand(deps.Client))

	return &Application{root: root}, nil
}

func newUnpairCommand(service ClientService) *cobra.Command {
	return &cobra.Command{
		Use:   "unpair",
		Short: "Remove this OS-user client's local endpoint identity",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if err := service.Unpair(); err != nil {
				return err
			}
			_, err := fmt.Fprintln(command.OutOrStdout(), "unpaired")
			return err
		},
	}
}

func newProtocolCommand(service ClientService) *cobra.Command {
	return &cobra.Command{
		Use:   "protocol <mga:// URI>",
		Short: "Handle an MGA protocol URI",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			parsed, err := url.Parse(args[0])
			if err != nil || parsed.Scheme != "mga" {
				return errors.New("unsupported MGA protocol URI")
			}
			if parsed.Host == "start" {
				return service.Start(command.Context(), clientapp.StartOptions{
					ServerURL: parsed.Query().Get("server"),
					LaunchID:  parsed.Query().Get("launch_id"),
					Token:     parsed.Query().Get("token"),
				})
			}
			if parsed.Host != "pair" {
				return errors.New("unsupported MGA protocol URI")
			}
			config, err := service.Pair(command.Context(), clientapp.PairOptions{
				ServerURL: parsed.Query().Get("server"),
				Code:      parsed.Query().Get("code"),
			})
			if err != nil {
				return err
			}
			if _, err = fmt.Fprintf(command.OutOrStdout(), "Paired %s as %s. Connecting agent…\n", config.DisplayName, config.EndpointID); err != nil {
				return err
			}
			return service.RunAgent(command.Context())
		},
	}
}

func newPairCommand(service ClientService) *cobra.Command {
	var options clientapp.PairOptions
	command := &cobra.Command{
		Use:   "pair",
		Short: "Pair this OS-user client with an MGA Server",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			config, err := service.Pair(command.Context(), options)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(command.OutOrStdout(), "Paired %s\nendpoint: %s\nserver: %s\n", config.DisplayName, config.EndpointID, config.ServerURL)
			return err
		},
	}
	command.Flags().StringVar(&options.ServerURL, "server", "", "MGA Server base URL (required)")
	command.Flags().StringVar(&options.Code, "code", "", "Single-use pairing code (required)")
	command.Flags().StringVar(&options.DisplayName, "name", "", "Device/user display name")
	_ = command.MarkFlagRequired("server")
	_ = command.MarkFlagRequired("code")
	return command
}

func newAgentCommand(service ClientService) *cobra.Command {
	return &cobra.Command{
		Use:   "agent",
		Short: "Run the per-user MGA device agent",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return service.RunAgent(command.Context())
		},
	}
}

func newStatusCommand(service ClientService) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show local pairing and endpoint identity status",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			status, err := service.Status()
			if err != nil {
				return err
			}
			if !status.Paired {
				_, err = fmt.Fprintln(command.OutOrStdout(), "not paired")
				return err
			}
			_, err = fmt.Fprintf(command.OutOrStdout(), "paired\nname: %s\nendpoint: %s\ninstance: %s\nserver: %s\nprivate_key: %t\n",
				status.DisplayName, status.EndpointID, status.ClientInstanceID, status.ServerURL, status.PrivateKeyReady)
			return err
		},
	}
}

func newDoctorCommand(service ClientService) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Validate local identity and MGA Server reachability",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			result, err := service.Doctor(command.Context())
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(command.OutOrStdout(), "paired: %t\nprivate_key: %t\nserver_health: %s\n",
				result.Status.Paired, result.Status.PrivateKeyReady, result.ServerHealth)
			return err
		},
	}
}

// Execute runs one CLI invocation using the supplied arguments.
func (a *Application) Execute(ctx context.Context, args []string) error {
	if a == nil || a.root == nil {
		return errors.New("CLI application is not initialized")
	}
	a.root.SetArgs(args)
	return a.root.ExecuteContext(ctx)
}

func newVersionCommand(info buildinfo.Info) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print client build and device protocol versions",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			rangeSupported := devicev1.SupportedVersionRange()
			_, err := fmt.Fprintf(
				command.OutOrStdout(),
				"mga-client %s\ncommit: %s\nbuild_date: %s\nprotocol: %d-%d\n",
				info.Version,
				info.Commit,
				info.BuildDate,
				rangeSupported.Min,
				rangeSupported.Max,
			)
			if err != nil {
				return fmt.Errorf("write version output: %w", err)
			}
			return nil
		},
	}
}
