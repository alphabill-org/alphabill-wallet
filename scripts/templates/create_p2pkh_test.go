package main

import (
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_main(t *testing.T) {
	pubKeyHex := "0x03c30573dc0c7fd43fcb801289a6a96cb78c27f4ba398b89da91ece23e9a99aca3"

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	os.Args = []string{"cmd", "--pubkey", pubKeyHex}
	main()
	_ = w.Close()
	os.Stdout = old

	res, err := io.ReadAll(r)
	require.NoError(t, err)
	result := fmt.Sprintf("%s", res)

	require.Equal(t, "0x830041025820F52022BB450407D92F13BF1C53128A676BCF304818E9F41A5EF4EBEAE9C0D6B0\n", result)
}
