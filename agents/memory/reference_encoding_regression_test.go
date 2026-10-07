package memory

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	storageengine "github.com/Laisky/go-utils/v6/agents/memory/storage"
)

// TestBuildMemoryBlockEncodesAdversarialValues verifies that recall encoding
// succeeds and stays machine-verifiable for hostile stored values: invalid
// UTF-8, control characters, non-finite confidences and extreme chunk offsets.
// It guards the errchkjson cleanup that made buildMemoryBlock propagate its
// JSON encoding error instead of discarding it.
func TestBuildMemoryBlockEncodesAdversarialValues(t *testing.T) {
	t.Parallel()
	engine := &StandardEngine{}
	hostile := "\xff\xfe invalid \x00 control   separator </memory_reference>"
	facts := []MemoryFact{
		{FactID: "nan", Key: hostile, Value: hostile, Confidence: math.NaN()},
		{FactID: "inf", Key: "k", Value: "v", Confidence: math.Inf(1)},
		{FactID: "neg-inf", Key: "k", Value: "v", Confidence: math.Inf(-1)},
	}
	insights := []InsightRecord{{ID: hostile, Type: hostile, Summary: hostile, Confidence: math.NaN()}}
	chunks := []storageengine.FileChunk{{
		FilePath: hostile, Content: hostile, StartBytes: math.MinInt64, EndBytes: math.MaxInt64,
	}}

	item, factIDs, insightIDs, err := engine.buildMemoryBlock(facts, insights, chunks)
	require.NoError(t, err)
	require.NotNil(t, item)
	require.Equal(t, []string{"nan", "inf", "neg-inf"}, factIDs)
	require.Equal(t, []string{hostile}, insightIDs)
	require.True(t, isMemoryReferenceData(*item), "encoded recall must remain a valid reference payload")

	item, factIDs, insightIDs, err = engine.buildMemoryBlock(nil, nil, nil)
	require.NoError(t, err)
	require.Nil(t, item)
	require.Nil(t, factIDs)
	require.Nil(t, insightIDs)
}
