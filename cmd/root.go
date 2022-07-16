// Package cmd some useful tools for command argument
package cmd

import (
	"math/rand"
	"time"

	"github.com/Laisky/go-utils/v2/config"
	"github.com/Laisky/go-utils/v2/log"
	"github.com/Laisky/zap"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "go-utils",
	Short: "go-utils",
	Long:  `go-utils`,
	Args:  NoExtraArgs,
	Run: func(cmd *cobra.Command, args []string) {

	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	defer func() {
		_ = log.Shared.Sync()
	}()
	rand.Seed(time.Now().UnixNano())

	var err error
	if err = config.Shared.BindPFlags(rootCmd.Flags()); err != nil {
		log.Shared.Panic("bind flags", zap.Error(err))
	}

	if config.Shared.GetBool("debug") {
		if err := log.Shared.ChangeLevel(log.LevelDebug); err != nil {
			log.Shared.Panic("change logger level to debug", zap.Error(err))
		}
	}

	if err = rootCmd.Execute(); err != nil {
		log.Shared.Panic("parse command line arguments", zap.Error(err))
	}
}

func init() {
	rootCmd.PersistentFlags().Bool("debug", false, "debug")
}

// NoExtraArgs make sure every args has been processed
//
// do not allow any unprocessed args
//
// Example
//
// use with cobra.Command:
//   cmd = &cobra.Command{
//       Args: NoExtraArgs,
//   }
func NoExtraArgs(cmd *cobra.Command, args []string) error {
	if len(args) != 0 {
		return errors.Errorf("unknown args `%v`", args)
	}

	return nil
}
