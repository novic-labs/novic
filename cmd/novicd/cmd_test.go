package main_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/cosmos-sdk/client/flags"
	svrcmd "github.com/cosmos/cosmos-sdk/server/cmd"
	"github.com/cosmos/cosmos-sdk/x/genutil/client/cli"

	"github.com/novic-labs/novic/v2/app"
	novicd "github.com/novic-labs/novic/v2/cmd/novicd"
)

func TestInitCmd(t *testing.T) {
	rootCmd, _ := novicd.NewRootCmd()
	rootCmd.SetArgs([]string{
		"init",      // Test the init cmd
		"novictest", // Moniker
		fmt.Sprintf("--%s=%s", cli.FlagOverwrite, "true"), // Overwrite genesis.json, in case it already exists
		fmt.Sprintf("--%s=%s", flags.FlagChainID, "novic_7000-1"),
	})

	err := svrcmd.Execute(rootCmd, "", app.DefaultNodeHome)
	require.NoError(t, err)
}
