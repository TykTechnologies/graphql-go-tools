package asyncapi

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAsyncAPIStreetLightsKafka(t *testing.T) {
	asyncapiDoc, err := os.ReadFile("./fixtures/streetlights-kafka.yaml")
	require.NoError(t, err)
	asyncapi, err := ParseAsyncAPIDocument(asyncapiDoc)
	require.NoError(t, err)

	fmt.Println(asyncapi, err)
}
