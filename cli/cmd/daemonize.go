package cmd

import (
	"fmt"
	"os"
	"syscall"

	"github.com/awnumar/memguard"
	"github.com/quexten/goldwarden/cli/agent"
	"github.com/spf13/cobra"
)

var daemonizeCmd = &cobra.Command{
	Use:   "daemonize",
	Short: "Starts the agent as a daemon",
	Long: `Starts the agent as a daemon. The agent will run in the background and will
	run in the background until it is stopped.`,
	Run: func(cmd *cobra.Command, args []string) {
		websocketDisabled := runtimeConfig.WebsocketDisabled
		sshDisabled := runtimeConfig.DisableSSHAgent

		if websocketDisabled {
			fmt.Println("Websocket disabled")
		}

		if sshDisabled {
			fmt.Println("SSH agent disabled")
		}

		cleanup := func() {
			fmt.Println("removing sockets and exiting")
			fmt.Println("unlinking", runtimeConfig.GoldwardenSocketPath)
			err := syscall.Unlink(runtimeConfig.GoldwardenSocketPath)
			if err != nil {
				fmt.Println(err)
			}
			fmt.Println("unlinking", runtimeConfig.SSHAgentSocketPath)
			err = syscall.Unlink(runtimeConfig.SSHAgentSocketPath)
			if err != nil {
				fmt.Println(err)
			}
			fmt.Println("memguard wiping memory and exiting")
			memguard.SafeExit(0)
		}

		home, _ := os.UserHomeDir()
		_, err := os.Stat("/.flatpak-info")
		isFlatpak := err == nil
		if runtimeConfig.GoldwardenSocketPath == "" {
			if isFlatpak {
				fmt.Println("Socket path is empty, overwriting with flatpak path.")
				runtimeConfig.GoldwardenSocketPath = home + "/.var/app/com.quexten.Goldwarden/data/goldwarden.sock"
			} else {
				fmt.Println("Socket path is empty, overwriting with default path.")
				runtimeConfig.GoldwardenSocketPath = home + "/.goldwarden.sock"
			}
		}
		if runtimeConfig.SSHAgentSocketPath == "" {
			if isFlatpak {
				fmt.Println("SSH Agent socket path is empty, overwriting with flatpak path.")
				runtimeConfig.SSHAgentSocketPath = home + "/.var/app/com.quexten.Goldwarden/data/ssh-auth-sock"
			} else {
				fmt.Println("SSH Agent socket path is empty, overwriting with default path.")
				runtimeConfig.SSHAgentSocketPath = home + "/.goldwarden-ssh-agent.sock"
			}
		}

		err = agent.StartUnixAgent(runtimeConfig.GoldwardenSocketPath, runtimeConfig)
		if err != nil {
			panic(err)
		}
		cleanup()
	},
}

func init() {
	rootCmd.AddCommand(daemonizeCmd)
}
