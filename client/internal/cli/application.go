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
	Pair(ctx context.Context, options clientapp.PairOptions) (clientconfig.Binding, error)
	Start(ctx context.Context, options clientapp.StartOptions) error
	RunAgent(ctx context.Context) error
	RunAgentReplacingExisting(ctx context.Context) error
	Status() (clientapp.Status, error)
	Doctor(ctx context.Context) (clientapp.DoctorResult, error)
	Unpair(options clientapp.UnpairOptions) error
	Installations() ([]clientapp.InstallationOwnershipRecord, error)
	ReleaseInstallation(options clientapp.ReleaseInstallationOptions) error
	AdoptInstallation(options clientapp.AdoptInstallationOptions) error
	ConfirmAndReleaseInstallation(ctx context.Context, options clientapp.ReleaseInstallationOptions) error
	ConfirmAndAdoptInstallation(ctx context.Context, options clientapp.AdoptInstallationOptions) error
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
	root.AddCommand(newInstallationsCommand(deps.Client))
	root.AddCommand(newProtocolCommand(deps.Client))

	return &Application{root: root}, nil
}

func newInstallationsCommand(service ClientService) *cobra.Command {
	command := &cobra.Command{Use: "installations", Short: "Manage local MGA installation ownership"}
	command.AddCommand(&cobra.Command{
		Use: "list", Short: "List locally managed installations", Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			items, err := service.Installations()
			if err != nil {
				return err
			}
			if len(items) == 0 {
				_, err = fmt.Fprintln(command.OutOrStdout(), "No locally managed installations.")
				return err
			}
			for _, item := range items {
				owner := item.OwnerBindingID
				if owner == "" {
					owner = "none"
				}
				if _, err = fmt.Fprintf(command.OutOrStdout(), "%s\n  title: %s\n  state: %s\n  owner: %s\n  path: %s\n", item.LocalInstallationID, item.Title, item.State, owner, item.InstallPath); err != nil {
					return err
				}
			}
			return nil
		},
	})
	var release clientapp.ReleaseInstallationOptions
	releaseCommand := &cobra.Command{
		Use: "release", Short: "Release an installation without deleting its files", Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if err := service.ReleaseInstallation(release); err != nil {
				return err
			}
			_, err := fmt.Fprintln(command.OutOrStdout(), "Installation released. Files were preserved.")
			return err
		},
	}
	releaseCommand.Flags().StringVar(&release.LocalInstallationID, "id", "", "Local installation ID (required)")
	releaseCommand.Flags().StringVar(&release.ServerURL, "server", "", "Confirm the current owning server URL")
	_ = releaseCommand.MarkFlagRequired("id")
	command.AddCommand(releaseCommand)
	var adopt clientapp.AdoptInstallationOptions
	adoptCommand := &cobra.Command{
		Use: "adopt", Short: "Give a released installation to a paired MGA Server", Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if err := service.AdoptInstallation(adopt); err != nil {
				return err
			}
			_, err := fmt.Fprintln(command.OutOrStdout(), "Installation adopted.")
			return err
		},
	}
	adoptCommand.Flags().StringVar(&adopt.LocalInstallationID, "id", "", "Local installation ID (required)")
	adoptCommand.Flags().StringVar(&adopt.ServerURL, "server", "", "Paired MGA Server URL that will own it (required)")
	_ = adoptCommand.MarkFlagRequired("id")
	_ = adoptCommand.MarkFlagRequired("server")
	command.AddCommand(adoptCommand)
	return command
}

func newUnpairCommand(service ClientService) *cobra.Command {
	var options clientapp.UnpairOptions
	command := &cobra.Command{
		Use:   "unpair",
		Short: "Remove this OS-user client's local endpoint identity",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if err := service.Unpair(options); err != nil {
				return err
			}
			_, err := fmt.Fprintln(command.OutOrStdout(), "unpaired")
			return err
		},
	}
	command.Flags().StringVar(&options.ServerURL, "server", "", "Unbind only this MGA Server URL")
	command.Flags().BoolVar(&options.All, "all", false, "Unbind every MGA Server")
	command.Flags().BoolVar(&options.ReleaseInstallations, "release-installations", false, "Preserve files and release this server's managed games for adoption")
	return command
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
			if parsed.Host == "release" || parsed.Host == "adopt" {
				localID, server := parsed.Query().Get("installation_id"), parsed.Query().Get("server")
				if parsed.Host == "release" {
					err = service.ConfirmAndReleaseInstallation(command.Context(), clientapp.ReleaseInstallationOptions{LocalInstallationID: localID, ServerURL: server})
				} else {
					err = service.ConfirmAndAdoptInstallation(command.Context(), clientapp.AdoptInstallationOptions{LocalInstallationID: localID, ServerURL: server})
				}
				if err != nil {
					return err
				}
				return service.RunAgentReplacingExisting(command.Context())
			}
			if parsed.Host == "start" {
				err = service.Start(command.Context(), clientapp.StartOptions{
					ServerURL:     parsed.Query().Get("server"),
					LaunchID:      parsed.Query().Get("launch_id"),
					Token:         parsed.Query().Get("token"),
					ExecutionMode: devicev1.ClientExecutionMode(parsed.Query().Get("mode")),
				})
				if errors.Is(err, clientapp.ErrElevationRelaunched) {
					return nil
				}
				return err
			}
			if parsed.Host != "pair" {
				return errors.New("unsupported MGA protocol URI")
			}
			_, err = service.Pair(command.Context(), clientapp.PairOptions{
				ServerURL: parsed.Query().Get("server"),
				Code:      parsed.Query().Get("code"),
			})
			if err != nil {
				return err
			}
			return service.RunAgentReplacingExisting(command.Context())
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
			if _, err = fmt.Fprintf(command.OutOrStdout(), "paired: %d server(s)\n", len(status.Bindings)); err != nil {
				return err
			}
			for _, binding := range status.Bindings {
				if _, err = fmt.Fprintf(command.OutOrStdout(), "\nname: %s\nbinding: %s\nendpoint: %s\ninstance: %s\nserver: %s\nprivate_key: %t\n",
					binding.DisplayName, binding.BindingID, binding.EndpointID, binding.ClientInstanceID, binding.ServerURL, binding.PrivateKeyReady); err != nil {
					return err
				}
			}
			return nil
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
			if _, err = fmt.Fprintf(command.OutOrStdout(), "paired: %t\nbindings: %d\n", result.Paired, len(result.Bindings)); err != nil {
				return err
			}
			for _, binding := range result.Bindings {
				if _, err = fmt.Fprintf(command.OutOrStdout(), "\nserver: %s\nprivate_key: %t\nserver_health: %s\n",
					binding.Status.ServerURL, binding.Status.PrivateKeyReady, binding.ServerHealth); err != nil {
					return err
				}
			}
			return nil
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
