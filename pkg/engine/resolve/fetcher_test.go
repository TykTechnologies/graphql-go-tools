package resolve

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/graphql-go-tools/pkg/fastbuffer"
)

// blockingDataSource lets a test deterministically overlap two concurrent
// fetches: Load reports it started (so the test knows both fetches are
// in-flight at once), then waits to be released before writing its own,
// distinct output.
type blockingDataSource struct {
	name    string
	started chan<- string
	release <-chan struct{}
}

func (d *blockingDataSource) Load(_ context.Context, _ []byte, w io.Writer) error {
	d.started <- d.name
	<-d.release
	_, err := w.Write([]byte(d.name))
	return err
}

// TestFetcherSingleFlightIncludesFetchIdentity guards against the single-flight
// in-flight key being derived only from the rendered input bytes. Two
// structurally different fetches (different SingleFetch/DataSource) that
// happen to render identical input must not be coalesced into one shared
// in-flight request - each must still call its own DataSource.Load and get
// back its own response.
func TestFetcherSingleFlightIncludesFetchIdentity(t *testing.T) {
	fetcher := NewFetcher(true)

	started := make(chan string, 2)
	release := make(chan struct{})

	input := fastbuffer.New()
	input.WriteString(`{"body":{"query":"{same}"}}`)

	fetches := []*SingleFetch{
		{DataSource: &blockingDataSource{name: "first", started: started, release: release}},
		{DataSource: &blockingDataSource{name: "second", started: started, release: release}},
	}
	outputs := []*BufPair{NewBufPair(), NewBufPair()}

	var wg sync.WaitGroup
	errs := make([]error, len(fetches))
	for i := range fetches {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = fetcher.Fetch(NewContext(context.Background()), fetches[i], input, outputs[i])
		}(i)
	}

	seen := map[string]bool{}
	for range fetches {
		select {
		case name := <-started:
			seen[name] = true
		case <-time.After(2 * time.Second):
			close(release)
			wg.Wait()
			t.Fatal("distinct fetches with identical rendered input were coalesced by single-flight - the second DataSource.Load never started")
		}
	}
	close(release)
	wg.Wait()

	for _, err := range errs {
		require.NoError(t, err)
	}

	assert.True(t, seen["first"] && seen["second"],
		"both distinct data sources must have been invoked, got: %v", seen)

	assert.Equal(t, "first", outputs[0].Data.String())
	assert.Equal(t, "second", outputs[1].Data.String())
}
