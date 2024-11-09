package main

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

type Blockchain struct {
	Name     string `json:"name"`
	RpcURL   string `json:"rpc_url"`
	ChainID  string `json:"chain_id"`
	Explorer string `json:"explorer"`
}

var blockchainFile = "blockchains.json"
var openAIAPIKey = "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"

func saveBlockchains(blockchains []Blockchain) error {
	jsonData, _ := json.MarshalIndent(blockchains, "", "  ")
	return ioutil.WriteFile(blockchainFile, jsonData, 0644)
}

func loadBlockchains() ([]Blockchain, error) {
	var blockchains []Blockchain
	file, err := ioutil.ReadFile(blockchainFile)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(file, &blockchains)
	return blockchains, err
}

func createTxLink(explorer, txHash string) string {
	parsedURL, _ := url.Parse(explorer)
	parsedURL.Path += "/tx/" + txHash
	return parsedURL.String()
}

func callOpenAI(prompt string) (string, error) {
	apiURL := "https://api.openai.com/v1/completions"

	requestBody, _ := json.Marshal(map[string]interface{}{
		"model":      "text-davinci-003",
		"prompt":     prompt,
		"max_tokens": 150,
	})

	req, _ := http.NewRequest("POST", apiURL, strings.NewReader(string(requestBody)))
	req.Header.Set("Authorization", "Bearer "+openAIAPIKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)
	var result map[string]interface{}
	json.Unmarshal(body, &result)

	if choices, ok := result["choices"].([]interface{}); ok && len(choices) > 0 {
		if text, ok := choices[0].(map[string]interface{})["text"].(string); ok {
			return text, nil
		}
	}
	return "", fmt.Errorf("Invalid OpenAI API response")
}

func deployContracts(privateKey string, blockchain Blockchain, col1 *fyne.Container, col2 *fyne.Container, gasLabel *widget.Label, wg *sync.WaitGroup) {
	defer wg.Done()

	client, err := ethclient.Dial(blockchain.RpcURL)
	if err != nil {
		col1.Add(widget.NewLabel(fmt.Sprintf("Connection error to %s : %v", blockchain.RpcURL, err)))
		col1.Refresh()
		return
	}

	pk, err := crypto.HexToECDSA(strings.TrimSpace(privateKey))
	if err != nil {
		col1.Add(widget.NewLabel(fmt.Sprintf("Error importing private key : %v", err)))
		col1.Refresh()
		return
	}

	publicKey := pk.Public()
	publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		col1.Add(widget.NewLabel("Public key conversion error"))
		col1.Refresh()
		return
	}

	fromAddress := crypto.PubkeyToAddress(*publicKeyECDSA)
	chainID, _ := new(big.Int).SetString(blockchain.ChainID, 10)
	nonce, err := client.PendingNonceAt(context.Background(), fromAddress)
	if err != nil {
		col1.Add(widget.NewLabel(fmt.Sprintf("Error retrieving nonce on %s : %v", blockchain.RpcURL, err)))
		col1.Refresh()
		return
	}

	gasPrice, err := client.SuggestGasPrice(context.Background())
	if err != nil {
		col1.Add(widget.NewLabel(fmt.Sprintf("Error when suggesting the gas price on %s : %v", blockchain.RpcURL, err)))
		col1.Refresh()
		return
	}

	bytecode := "0x6080604052348015600f57600080fd5b5060a08061001d6000396000f3fe608060405260043610603f5760003560e01c80636057361d1460445780636d4ce63c14605e575b600080fd5b605c60048036036020811015605857600080fd5b50356067565b005b348015606957600080fd5b5060706073565b005b56fea26469706673582212207e450dcde54ac92df0b002d3cf04dd1d2331d7685cf4be5f2b5e2dc5dbedcbe564736f6c63430008090033"
	totalGasUsed := big.NewInt(0)

	for i := 0; i < 10; i++ {
		tx := types.NewContractCreation(nonce, big.NewInt(0), 2000000, gasPrice, common.FromHex(bytecode))
		signedTx, err := types.SignTx(tx, types.LatestSignerForChainID(chainID), pk)
		if err != nil {
			col1.Add(widget.NewLabel(fmt.Sprintf("Transaction signature error %d on %s : %v", i+1, blockchain.RpcURL, err)))
			col1.Refresh()
			return
		}

		err = client.SendTransaction(context.Background(), signedTx)
		if err != nil {
			col1.Add(widget.NewLabel(fmt.Sprintf("Error sending transaction %d on %s : %v", i+1, blockchain.RpcURL, err)))
			col1.Refresh()
			return
		}

		txHash := signedTx.Hash().Hex()
		txReceipt, err := bind.WaitMined(context.Background(), client, signedTx)
		if err != nil {
			col1.Add(widget.NewLabel(fmt.Sprintf("Error receiving transaction %d : %v", i+1, err)))
			col1.Refresh()
			return
		}

		gasUsed := new(big.Int).SetUint64(txReceipt.GasUsed)
		totalGasUsed.Add(totalGasUsed, gasUsed)

		link := createTxLink(blockchain.Explorer, txHash)
		parsedURL, _ := url.Parse(link)
		txLink := widget.NewHyperlink(fmt.Sprintf("Tx %d", i+1), parsedURL)

		// Ajouter les transactions à la première colonne jusqu'à 5, puis à la deuxième colonne
		if i < 5 {
			col1.Add(txLink)
			col1.Refresh()
		} else {
			col2.Add(txLink)
			col2.Refresh()
		}

		time.Sleep(500 * time.Millisecond)
		nonce++
	}

	gasLabel.SetText(fmt.Sprintf("Total gas costs used : %s gas units", totalGasUsed.String()))
	col1.Refresh()
	col2.Refresh()
}

func animateText(label *widget.Label, text string) {
	label.SetText("")
	go func() {
		for i := 0; i <= len(text); i++ {
			label.SetText(text[:i])
			time.Sleep(50 * time.Millisecond)
		}
	}()
}

func main() {
	myApp := app.New()
	myWindow := myApp.NewWindow("Multid")
	myWindow.Resize(fyne.NewSize(800, 600))
	myWindow.SetFixedSize(true)

	privateKeyEntry := widget.NewPasswordEntry()
	privateKeyEntry.SetPlaceHolder("Enter your private key")

	blockchainList := widget.NewSelect([]string{}, nil)
	col1 := container.NewVBox()
	col2 := container.NewVBox()
	columns := container.NewHBox(col1, col2)
	gasLabel := widget.NewLabel("Total gas costs used :")

	blockchains, err := loadBlockchains()
	if err == nil {
		names := []string{}
		for _, bc := range blockchains {
			names = append(names, bc.Name)
		}
		blockchainList.Options = names
		blockchainList.Refresh()
	}

	helpLabel := widget.NewLabel("")
	helpLabel.Wrapping = fyne.TextWrapWord

	helpButton := widget.NewButton("How to use this tool ?", func() {
		explanation := "To use this tool, select a blockchain, enter your private key and click on 'Deploy'.'."
		animateText(helpLabel, explanation)
	})

	askAIButton := widget.NewButton("Ask the AI", func() {
		prompt := "What are the best parameters for deploying a contract on " + blockchainList.Selected + " ?"
		answer, err := callOpenAI(prompt)
		if err != nil {
			col1.Add(widget.NewLabel(fmt.Sprintf("AI error : %v", err)))
		} else {
			helpLabel.SetText("IA : " + answer)
		}
		col1.Refresh()
	})

	addBlockchainButton := widget.NewButton("Add Blockchain", func() {
		nameEntry := widget.NewEntry()
		nameEntry.SetPlaceHolder("Name")
		rpcEntry := widget.NewEntry()
		rpcEntry.SetPlaceHolder("URL RPC")
		chainIDEntry := widget.NewEntry()
		chainIDEntry.SetPlaceHolder("Chain ID")
		explorerEntry := widget.NewEntry()
		explorerEntry.SetPlaceHolder("URL Exploror")

		form := widget.NewForm(
			widget.NewFormItem("Name", nameEntry),
			widget.NewFormItem("RPC", rpcEntry),
			widget.NewFormItem("ID chain", chainIDEntry),
			widget.NewFormItem("Exploror", explorerEntry),
		)

		dialog := dialog.NewForm("Add Blockchain", "Add", "Cancel", form.Items, func(ok bool) {
			if ok {
				blockchain := Blockchain{
					Name:     nameEntry.Text,
					RpcURL:   rpcEntry.Text,
					ChainID:  chainIDEntry.Text,
					Explorer: explorerEntry.Text,
				}
				blockchains = append(blockchains, blockchain)
				saveBlockchains(blockchains)
				blockchainList.Options = append(blockchainList.Options, blockchain.Name)
				blockchainList.Refresh()
			}
		}, myWindow)
		dialog.Show()
	})

	deleteBlockchainButton := widget.NewButton("Remove Blockchain", func() {
		selected := blockchainList.Selected
		if selected == "" {
			dialog.ShowInformation("Info", "Please select a blockchain to remove.", myWindow)
			return
		}

		for i, bc := range blockchains {
			if bc.Name == selected {
				blockchains = append(blockchains[:i], blockchains[i+1:]...)
				saveBlockchains(blockchains)
				names := []string{}
				for _, b := range blockchains {
					names = append(names, b.Name)
				}
				blockchainList.Options = names
				blockchainList.Refresh()
				dialog.ShowInformation("Info", fmt.Sprintf("La blockchain '%s' has been removed.", selected), myWindow)
				break
			}
		}
	})

	modifyBlockchainButton := widget.NewButton("Edit Blockchain", func() {
		selected := blockchainList.Selected
		if selected == "" {
			dialog.ShowInformation("Info", "Please select a blockchain to modify.", myWindow)
			return
		}

		var blockchain Blockchain
		for _, bc := range blockchains {
			if bc.Name == selected {
				blockchain = bc
				break
			}
		}

		nameEntry := widget.NewEntry()
		nameEntry.SetText(blockchain.Name)
		rpcEntry := widget.NewEntry()
		rpcEntry.SetText(blockchain.RpcURL)
		chainIDEntry := widget.NewEntry()
		chainIDEntry.SetText(blockchain.ChainID)
		explorerEntry := widget.NewEntry()
		explorerEntry.SetText(blockchain.Explorer)

		form := widget.NewForm(
			widget.NewFormItem("Nom", nameEntry),
			widget.NewFormItem("RPC", rpcEntry),
			widget.NewFormItem("ID de Chaîne", chainIDEntry),
			widget.NewFormItem("Explorateur", explorerEntry),
		)

		dialog := dialog.NewForm("Edit Blockchain", "Edit", "Cancel", form.Items, func(ok bool) {
			if ok {
				for i, bc := range blockchains {
					if bc.Name == selected {
						blockchains[i] = Blockchain{
							Name:     nameEntry.Text,
							RpcURL:   rpcEntry.Text,
							ChainID:  chainIDEntry.Text,
							Explorer: explorerEntry.Text,
						}
						saveBlockchains(blockchains)
						break
					}
				}

				names := []string{}
				for _, b := range blockchains {
					names = append(names, b.Name)
				}
				blockchainList.Options = names
				blockchainList.Refresh()
				dialog.ShowInformation("Info", fmt.Sprintf("La blockchain '%s' has been modified.", selected), myWindow)
			}
		}, myWindow)
		dialog.Show()
	})

	deployButton := widget.NewButton("Deploy", func() {
		selected := blockchainList.Selected
		if selected == "" {
			col1.Add(widget.NewLabel("Select a blockchain"))
			col1.Refresh()
			return
		}

		var blockchain Blockchain
		for _, bc := range blockchains {
			if bc.Name == selected {
				blockchain = bc
				break
			}
		}

		privateKey := strings.TrimSpace(privateKeyEntry.Text)
		if privateKey == "" {
			col1.Add(widget.NewLabel("Please enter a private key"))
			col1.Refresh()
			return
		}

		col1.Objects = nil
		col2.Objects = nil
		col1.Refresh()
		col2.Refresh()

		var wg sync.WaitGroup
		wg.Add(1)
		go deployContracts(privateKey, blockchain, col1, col2, gasLabel, &wg)
		wg.Wait()

		col1.Add(widget.NewLabel(fmt.Sprintf("Deployment completed on %s.", blockchain.Name)))
		col1.Refresh()
	})

	content := container.NewVBox(
		helpButton,
		askAIButton,
		widget.NewLabel("Deploying contracts on multiple blockchains"),
		privateKeyEntry,
		blockchainList,
		addBlockchainButton,
		modifyBlockchainButton,
		deleteBlockchainButton,
		deployButton,
		gasLabel,
		helpLabel,
		columns,
	)

	myWindow.SetContent(content)
	myWindow.ShowAndRun()
}
