package apis

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ledgerwatch/turbo-geth/common"
	"github.com/ledgerwatch/turbo-geth/common/dbutils"
	"github.com/ledgerwatch/turbo-geth/ethdb"
)

func RegisterStorageTombstonesAPI(router *gin.RouterGroup, e *Env) error {
	router.GET("/", e.FindStorageTombstone)
	return nil
}

func (e *Env) FindStorageTombstone(c *gin.Context) {
	results, err := findStorageTombstoneByPrefix(c.Query("prefix"), e.DB)
	if err != nil {
		c.Error(err) //nolint:errcheck
		return
	}
	c.JSON(http.StatusOK, results)
}

type StorageTombsResponse struct {
	Prefix string `json:"prefix"`
}

func findStorageTombstoneByPrefix(prefixS string, remoteDB ethdb.KV) ([]*StorageTombsResponse, error) {
	var results []*StorageTombsResponse
	prefix := common.FromHex(prefixS)
	if err := remoteDB.View(context.TODO(), func(tx ethdb.Tx) error {
		interBucket := tx.Bucket(dbutils.IntermediateTrieHashBucket)
		c := interBucket.Cursor().Prefix(prefix).NoValues()

		for k, vSize, err := c.First(); k != nil || err != nil; k, vSize, err = c.Next() {
			if err != nil {
				return err
			}

			if vSize > 0 {
				continue
			}

			results = append(results, &StorageTombsResponse{
				Prefix: fmt.Sprintf("%x\n", k),
			})

			if len(results) > 50 {
				results = append(results, &StorageTombsResponse{
					Prefix: "too much results",
				})
				return nil
			}
		}

		return nil
	}); err != nil {
		return nil, err
	}

	return results, nil
}
