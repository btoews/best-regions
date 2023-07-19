package main

import (
	"bytes"
	"math"
	"testing"

	"github.com/alecthomas/assert/v2"
)

func TestDecodePromData(t *testing.T) {
	const d = `{"status":"success","isPartial":false,"data":{"resultType":"vector","result":[{"metric":{"region":"ams"},"value":[1689867197,"21"]},{"metric":{"region":"arn"},"value":[1689867197,"20"]},{"metric":{"region":"atl"},"value":[1689867197,"4"]},{"metric":{"region":"bom"},"value":[1689867197,"12"]},{"metric":{"region":"cdg"},"value":[1689867197,"31"]},{"metric":{"region":"chi"},"value":[1689867197,"5"]},{"metric":{"region":"dfw"},"value":[1689867197,"32"]},{"metric":{"region":"fra"},"value":[1689867197,"85"]},{"metric":{"region":"gdl"},"value":[1689867197,"2"]},{"metric":{"region":"gru"},"value":[1689867197,"51"]},{"metric":{"region":"hkg"},"value":[1689867197,"33"]},{"metric":{"region":"iad"},"value":[1689867197,"19"]},{"metric":{"region":"jnb"},"value":[1689867197,"8"]},{"metric":{"region":"lax"},"value":[1689867197,"47"]},{"metric":{"region":"lga"},"value":[1689867197,"25"]},{"metric":{"region":"yyz"},"value":[1689867197,"26"]}]}}`
	pd, err := readPromData(bytes.NewReader([]byte(d)))
	assert.NoError(t, err)
	assert.Equal(t, promData{
		"ams": 21,
		"arn": 20,
		"atl": 4,
		"bom": 12,
		"cdg": 31,
		"chi": 5,
		"dfw": 32,
		"fra": 85,
		"gdl": 2,
		"gru": 51,
		"hkg": 33,
		"iad": 19,
		"jnb": 8,
		"lax": 47,
		"lga": 25,
		"yyz": 26,
	}, pd)
}

func TestModelParams(t *testing.T) {
	vertices, edgeCosts := modelParams(map[string]map[string]int{
		"a": {"b": 2},
		"b": {"a": 1},
	})
	assert.Equal(t, []string{"a", "b"}, vertices)
	assert.Equal(t, [][]float64{{1.5}}, edgeCosts)

	vertices, edgeCosts = modelParams(map[string]map[string]int{
		"a": {"b": 2, "c": 3},
		"b": {"a": 1},
	})
	assert.Equal(t, []string{"a", "b", "c"}, vertices)
	assert.Equal(t, [][]float64{{1.5}, {3, math.MaxFloat64}}, edgeCosts)

	vertices, edgeCosts = modelParams(map[string]map[string]int{
		"a": {"b": 2},
		"b": {"a": 1},
		"c": {"a": 3, "b": 4},
	})
	assert.Equal(t, []string{"a", "b", "c"}, vertices)
	assert.Equal(t, [][]float64{{1.5}, {3, 4}}, edgeCosts)
}
