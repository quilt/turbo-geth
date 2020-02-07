package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ledgerwatch/turbo-geth/common"
	"github.com/ledgerwatch/turbo-geth/common/dbutils"
	"github.com/ledgerwatch/turbo-geth/core/types/accounts"
	"github.com/ledgerwatch/turbo-geth/ethdb/remote"
)

func registerAccountAPI(account *gin.RouterGroup, remoteDB *remote.DB) error {
	fmt.Println("remote db connected")

	account.GET(":accountID", func(c *gin.Context) {
		account, err := FindAccountByID(c.Param("accountID"), remoteDB)
		if err == ErrEntityNotFound {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"message": "account not found"})
			return
		} else if err != nil {
			c.AbortWithError(http.StatusInternalServerError, err)
			return
		}
		c.JSON(http.StatusOK, jsonifyAccount(account))
	})
	return nil
}

func jsonifyAccount(account *accounts.Account) map[string]interface{} {
	result := map[string]interface{}{
		"nonce":     account.Nonce,
		"balance":   account.Balance.String(),
		"root_hash": account.Root.Hex(),
		"code_hash": account.CodeHash.Hex(),
		"implementation": map[string]interface{}{
			"incarnation": account.Incarnation,
		},
	}

	return result
}

func FindAccountByID(accountID string, remoteDB *remote.DB) (*accounts.Account, error) {

	possibleKeys := getPossibleKeys(accountID)

	var account *accounts.Account

	err := remoteDB.View(context.TODO(), func(tx *remote.Tx) error {
		bucket, err := tx.Bucket(dbutils.AccountsBucket)
		if err != nil {
			return err
		}

		for _, key := range possibleKeys {
			accountRlp, err := bucket.Get(key)
			if len(accountRlp) == 0 {
				continue
			}
			if err != nil {
				return err
			}

			account = &accounts.Account{}
			return account.DecodeForStorage(accountRlp)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	if account == nil {
		return nil, ErrEntityNotFound
	}

	return account, nil
}

func getPossibleKeys(accountID string) [][]byte {
	address := common.FromHex(accountID)
	addressHash, _ := common.HashData(address[:])
	return [][]byte{
		addressHash[:],
		address,
	}
}