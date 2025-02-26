package jetton

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/moorzeen/common-go/logger"
	"github.com/patrickmn/go-cache"
	"github.com/sirupsen/logrus"
	"github.com/xssnick/tonutils-go/address"
	"github.com/xssnick/tonutils-go/ton"
	"github.com/xssnick/tonutils-go/ton/jetton"
	"github.com/xssnick/tonutils-go/ton/nft"
	"github.com/xssnick/tonutils-go/tvm/cell"
)

const (
	contentOnchain   = "onchain"
	contentSemichain = "semichain"
	contentOffchain  = "offchain"
)

var memcache = cache.New(5*time.Minute, 10*time.Minute)

type MasterData struct {
	Address     *address.Address
	ContentType string
	Name        string
	Symbol      string
	Description string
	Image       string
	Decimals    int
}

type OffchainContent struct {
	Name        string `json:"name"`
	Symbol      string `json:"symbol"`
	Description string `json:"description"`
	Image       string `json:"image"`
	Decimals    int32  `json:"decimals"`
}

func GetMasterData(ctx context.Context, api ton.APIClientWrapped, master *address.Address) (*MasterData, error) {
	mc := jetton.NewJettonMasterClient(api, master)

	data, err := mc.GetJettonData(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get jetton data: %w", err)
	}

	var contentType, name, symbol, description, image, decimals string

	switch data.Content.(type) {

	case *nft.ContentOnchain:
		content := data.Content.(*nft.ContentOnchain)
		contentType = contentOnchain
		name = content.GetAttribute("name")
		symbol = content.GetAttribute("symbol")
		description = content.GetAttribute("description")
		image = content.GetAttribute("image")
		decimals = content.GetAttribute("decimals")

	case *nft.ContentSemichain:
		content := data.Content.(*nft.ContentSemichain)
		contentType = contentSemichain

		result, err := cachedOffchainContent(content.URI)
		if err != nil {
			logrus.Errorf("fetch cashed offchain content: %s", err)
			break
		}

		name = result.Name
		symbol = result.Symbol
		description = result.Description
		image = result.Image
		decimals = content.GetAttribute("decimals")

	case *nft.ContentOffchain:
		content := data.Content.(*nft.ContentOffchain)
		contentType = contentOffchain

		result, err := cachedOffchainContent(content.URI)
		if err != nil {
			logrus.Errorf("fetch cashed offchain content: %s", err)
			break
		}

		name = result.Name
		symbol = result.Symbol
		description = result.Description
		image = result.Image
		decimals = string(result.Decimals)

	default:
		logrus.Error("unknown content type")
	}

	dec, err := strconv.Atoi(decimals)
	if err != nil {
		logrus.Errorf("convert decimals: %s", err)
	}

	return &MasterData{
		Address:     master,
		ContentType: contentType,
		Name:        name,
		Symbol:      symbol,
		Description: description,
		Image:       image,
		Decimals:    dec,
	}, err
}

func GetMasterByWallet(ctx context.Context, api ton.APIClientWrapped, jettonWallet *address.Address) (*MasterData, error) {
	b, err := api.CurrentMasterchainInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("get current master chain info: %w", err)
	}

	res, err := api.RunGetMethod(ctx, b, jettonWallet, "get_wallet_data")
	if err != nil {
		return nil, fmt.Errorf("run get method: %w", err)
	}

	master := &address.Address{}

	for _, c := range res.AsTuple() {
		switch res := c.(type) {
		case *cell.Slice:
			master, err = res.LoadAddr()
			if err != nil {
				return nil, fmt.Errorf("load master address: %w", err)
			}
		default:

		}
	}

	data, err := GetMasterData(ctx, api, master)
	if err != nil {
		return nil, fmt.Errorf("failed to get master data: %w", err)
	}

	return data, nil
}

func cachedOffchainContent(uri string) (*OffchainContent, error) {
	if cached, ok := memcache.Get(uri); ok {
		fmt.Println("got from cache", logger.AnyPrint(cached))
		return cached.(*OffchainContent), nil
	}

	result, err := fetchOffchainContent(uri)
	if err != nil {
		return nil, fmt.Errorf("fetch offchain content: %w", err)
	}

	memcache.Set(uri, result, time.Hour)
	fmt.Println("set cache", logger.AnyPrint(result))

	return result, nil
}

func fetchOffchainContent(uri string) (*OffchainContent, error) {
	resp, err := http.Get(uri)
	if err != nil {
		return nil, fmt.Errorf("do get request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected response status code: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	result := &OffchainContent{}

	err = json.Unmarshal(body, result)
	if err != nil {
		return nil, fmt.Errorf("unmarshal response body: %w", err)
	}

	return result, nil
}
