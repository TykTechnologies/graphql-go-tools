package postprocess

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/TykTechnologies/graphql-go-tools/pkg/engine/plan"
	"github.com/TykTechnologies/graphql-go-tools/pkg/engine/resolve"
)

func TestProcessHeaderModifier_Process(t *testing.T) {
	modifier := func(header http.Header) {
		header.Add("X-Test-Header", "test value")
	}
	processor := NewProcessHeaderModifier(modifier)

 header, _ := http.ReadRequest(bufio.NewReader(bytes.NewReader(fetch.Input)))
 header.Add("X-Test-Header", "test value")
 buf := new(bytes.Buffer)
 buf.Write(header)
 pre := &plan.SynchronousResponsePlan{
 	Response: &resolve.GraphQLResponse{
 		Data: &resolve.Object{
 			Fetch: &resolve.SingleFetch{
 				BufferId:   0,
 				Input:      buf.Bytes(),
 				DataSource: nil,
 			},
 		},
 	},
 }

	post := processor.Process(pre)
	postResponse, ok := post.(*plan.SynchronousResponsePlan)
	assert.True(t, ok)

 	fetch := postResponse.Response.Data.(*resolve.Object).Fetch.(*resolve.SingleFetch)
 	assert.Equal(t, "test value", http.Header(fetch.Input).Get("X-Test-Header"))
}

