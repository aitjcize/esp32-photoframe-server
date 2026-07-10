package service

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeTopicFetch returns a fetch function serving `total` results with
// page-scoped ids, recording which pages were requested.
func fakeTopicFetch(perPage, total int, pagesFetched *[]int) func(page int) ([]RemoteAsset, int, error) {
	return func(page int) ([]RemoteAsset, int, error) {
		*pagesFetched = append(*pagesFetched, page)
		start := (page - 1) * perPage
		var out []RemoteAsset
		for i := start; i < start+perPage && i < total; i++ {
			out = append(out, RemoteAsset{ExternalID: fmt.Sprintf("id-%d", i)})
		}
		return out, total, nil
	}
}

func TestSearchTopicPagesSequential(t *testing.T) {
	var pages []int
	assets, err := searchTopicPages(10, 25, false, fakeTopicFetch(10, 1000, &pages))
	require.NoError(t, err)
	assert.Len(t, assets, 25)
	assert.Equal(t, []int{1, 2, 3}, pages, "sequential mode reads pages in order")
	assert.Equal(t, "id-0", assets[0].ExternalID)
}

func TestSearchTopicPagesSequentialShortResults(t *testing.T) {
	var pages []int
	assets, err := searchTopicPages(10, 100, false, fakeTopicFetch(10, 15, &pages))
	require.NoError(t, err)
	assert.Len(t, assets, 15, "stops when results run out")
}

func TestSearchTopicPagesRandomized(t *testing.T) {
	var pages []int
	// 5000 results, perPage 10 → sampling window capped at 1000 results = 100 pages.
	assets, err := searchTopicPages(10, 30, true, fakeTopicFetch(10, 5000, &pages))
	require.NoError(t, err)
	assert.Len(t, assets, 30)

	require.NotEmpty(t, pages)
	assert.Equal(t, 1, pages[0], "page 1 is always fetched first")
	seenPages := map[int]bool{}
	for _, p := range pages {
		assert.False(t, seenPages[p], "page %d fetched twice", p)
		seenPages[p] = true
		assert.LessOrEqual(t, p, 100, "sampling must stay within the first 1000 results")
		assert.GreaterOrEqual(t, p, 1)
	}

	// No duplicate assets even though pages arrive out of order.
	seenIDs := map[string]bool{}
	for _, a := range assets {
		assert.False(t, seenIDs[a.ExternalID], "asset %s duplicated", a.ExternalID)
		seenIDs[a.ExternalID] = true
	}
}

// With few results, randomize degrades to fetching everything once.
func TestSearchTopicPagesRandomizedSmallPool(t *testing.T) {
	var pages []int
	assets, err := searchTopicPages(10, 100, true, fakeTopicFetch(10, 8, &pages))
	require.NoError(t, err)
	assert.Len(t, assets, 8)
	assert.Equal(t, []int{1}, pages, "a single-page result set needs no sampling")
}
