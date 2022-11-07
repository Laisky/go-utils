// Package cmd some useful tools for command argument
package cmd

import (
	"math/rand"
	"time"

	"github.com/Laisky/errors"
	"github.com/Laisky/zap"
	"github.com/spf13/cobra"

	"github.com/Laisky/go-utils/v2/log"
)

var (
	cmdDebug bool
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

	if cmdDebug {
		if err := log.Shared.ChangeLevel(log.LevelDebug); err != nil {
			log.Shared.Panic("change logger level to debug", zap.Error(err))
		}
	}

	if err := rootCmd.Execute(); err != nil {
		log.Shared.Panic("parse command line arguments", zap.Error(err))
	}
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&cmdDebug, "debug", false, "debug")
}

// NoExtraArgs make sure every args has been processed
//
// do not allow any unprocessed args
//
// # Example
//
// use with cobra.Command:
//
//	cmd = &cobra.Command{
//	    Args: NoExtraArgs,
//	}
func NoExtraArgs(cmd *cobra.Command, args []string) error {
	if len(args) != 0 {
		return errors.Errorf("unknown args `%v`", args)
	}

	return nil
}
