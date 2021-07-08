package cmd

import (
	"fmt"
	"math/rand"
	"time"

	gutils "github.com/Laisky/go-utils"
	"github.com/Laisky/zap"
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
		_ = gutils.Logger.Sync()
	}()
	rand.Seed(time.Now().UnixNano())

	var err error
	if err = gutils.Settings.BindPFlags(rootCmd.Flags()); err != nil {
		gutils.Logger.Panic("bind flags", zap.Error(err))
	}

	if gutils.Settings.GetBool("debug") {
		if err := gutils.Logger.ChangeLevel(gutils.LoggerLevelDebug); err != nil {
			gutils.Logger.Panic("change logger level to debug", zap.Error(err))
		}
	}

	if err = rootCmd.Execute(); err != nil {
		gutils.Logger.Panic("parse command line arguments", zap.Error(err))
	}
}

func init() {
	rootCmd.PersistentFlags().Bool("debug", false, "debug")
}

// NoExtraArgs make sure every args has been processed
//
// do not allow any un processed args
func NoExtraArgs(cmd *cobra.Command, args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("unknown args `%v`", args)
	}

	return nil
}
