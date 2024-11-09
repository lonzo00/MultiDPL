package main

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math/big"
	"net/url"
	"strings"

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
	// Assurez-vous d'inclure le package ABI de votre contrat NFT compilé
)

// Structure pour la configuration de la blockchain
type Blockchain struct {
	Name     string `json:"name"`
	RpcURL   string `json:"rpc_url"`
	ChainID  string `json:"chain_id"`
	Explorer string `json:"explorer"`
}

var blockchainFile = "blockchains.json"

// Fonction pour charger les configurations blockchain
func loadBlockchains() ([]Blockchain, error) {
	var blockchains []Blockchain
	file, err := ioutil.ReadFile(blockchainFile)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(file, &blockchains)
	return blockchains, err
}

// Fonction pour créer un lien d'explorateur
func createExplorerLink(explorerURL, pathType, hash string) string {
	return fmt.Sprintf("%s/%s/%s", strings.TrimRight(explorerURL, "/"), pathType, hash)
}

// Fonction pour obtenir la clé privée depuis l'interface utilisateur
func getPrivateKeyFromInput(keyEntry *widget.Entry) (*ecdsa.PrivateKey, error) {
	privateKeyHex := strings.TrimSpace(keyEntry.Text)
	if privateKeyHex == "" {
		return nil, fmt.Errorf("Veuillez entrer votre clé privée")
	}

	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		return nil, fmt.Errorf("Clé privée invalide : %v", err)
	}

	return privateKey, nil
}

// Fonction pour envoyer une transaction ETH
func sendEthTransaction(client *ethclient.Client, auth *bind.TransactOpts, to common.Address, amount *big.Int) (*types.Transaction, error) {
	tx := types.NewTransaction(auth.Nonce.Uint64(), to, amount, auth.GasLimit, auth.GasPrice, nil)

	err := client.SendTransaction(context.Background(), tx)
	if err != nil {
		return nil, fmt.Errorf("Erreur lors de l'envoi de la transaction : %v", err)
	}

	fmt.Printf("Transaction envoyée : %s\n", tx.Hash().Hex())
	return tx, nil
}

// Fonction pour déployer un contrat NFT
func deployNFTContract(client *ethclient.Client, auth *bind.TransactOpts, name, symbol string) (*types.Transaction, error) {
	// Exemple d'appel de contrat pour un NFT ERC-721 standard
	// Remplacez ceci par le déploiement du contrat réel

	auth.GasLimit = uint64(3000000) // Augmentez cette valeur si nécessaire

	// Création de la transaction pour déployer le contrat NFT
	// Remplacez "NFTContractABI" et "NFTContractBytecode" par ceux du contrat compilé
	// Example: instance, tx, _, err := myNFTContract.DeployNFT(auth, client, name, symbol)
	tx := types.NewContractCreation(auth.Nonce.Uint64(), big.NewInt(0), auth.GasLimit, auth.GasPrice, nil)

	err := client.SendTransaction(context.Background(), tx)
	if err != nil {
		return nil, fmt.Errorf("Erreur lors du déploiement du contrat NFT : %v", err)
	}

	fmt.Printf("Contrat NFT '%s' (%s) déployé : %s\n", name, symbol, tx.Hash().Hex())
	return tx, nil
}

func main() {
	// Charger les configurations blockchain
	blockchains, err := loadBlockchains()
	if err != nil {
		log.Fatalf("Erreur de chargement des configurations blockchain : %v", err)
	}

	// Initialiser l'application Fyne
	myApp := app.New()
	myWindow := myApp.NewWindow("Outil Blockchain")

	// Créer une zone de texte pour afficher les résultats avec un lien hypertexte
	feedbackLabel := widget.NewLabel("")
	feedbackScroll := container.NewVScroll(feedbackLabel)
	feedbackScroll.SetMinSize(fyne.NewSize(380, 100))

	// Liste déroulante pour sélectionner la blockchain
	blockchainNames := []string{}
	for _, b := range blockchains {
		blockchainNames = append(blockchainNames, b.Name)
	}
	blockchainSelector := widget.NewSelect(blockchainNames, func(value string) {})
	blockchainSelector.PlaceHolder = "Sélectionner une Blockchain"

	// Champs pour le transfert ETH
	recipientEntry := widget.NewEntry()
	recipientEntry.SetPlaceHolder("Adresse du destinataire (ex: 0x123...)")
	amountEntry := widget.NewEntry()
	amountEntry.SetPlaceHolder("Montant en ETH (ex: 0.01)")

	// Champs pour le NFT
	nameEntry := widget.NewEntry()
	nameEntry.SetPlaceHolder("Nom du NFT")
	symbolEntry := widget.NewEntry()
	symbolEntry.SetPlaceHolder("Symbole du NFT")

	// Champ pour la clé privée
	privateKeyEntry := widget.NewPasswordEntry()
	privateKeyEntry.SetPlaceHolder("Collez votre clé privée ici")

	// Bouton pour envoyer une transaction ETH
	sendButton := widget.NewButton("Envoyer Transaction ETH", func() {
		// Code pour envoyer la transaction ETH
		// ...
	})

	// Bouton pour déployer le contrat NFT
	deployNFTButton := widget.NewButton("Déployer NFT", func() {
		// Vérification de la sélection d'une blockchain
		selectedName := blockchainSelector.Selected
		var selectedBlockchain Blockchain
		for _, b := range blockchains {
			if b.Name == selectedName {
				selectedBlockchain = b
				break
			}
		}
		if selectedBlockchain.RpcURL == "" {
			dialog.ShowError(fmt.Errorf("Veuillez sélectionner une blockchain"), myWindow)
			return
		}

		// Connexion à la blockchain
		client, err := ethclient.Dial(selectedBlockchain.RpcURL)
		if err != nil {
			dialog.ShowError(fmt.Errorf("Échec de la connexion au client : %v", err), myWindow)
			return
		}
		defer client.Close()

		// Obtenir la clé privée
		privateKey, err := getPrivateKeyFromInput(privateKeyEntry)
		if err != nil {
			dialog.ShowError(err, myWindow)
			return
		}

		// Configurer l’authentification avec la clé privée
		chainID := new(big.Int)
		chainID.SetString(selectedBlockchain.ChainID, 10)
		auth, err := bind.NewKeyedTransactorWithChainID(privateKey, chainID)
		if err != nil {
			dialog.ShowError(fmt.Errorf("Erreur de création du transactor : %v", err), myWindow)
			return
		}

		// Déployer le NFT
		nftName := strings.TrimSpace(nameEntry.Text)
		nftSymbol := strings.TrimSpace(symbolEntry.Text)
		tx, err := deployNFTContract(client, auth, nftName, nftSymbol)
		if err != nil {
			dialog.ShowError(fmt.Errorf("Échec du déploiement du contrat NFT : %v", err), myWindow)
			return
		}

		// Afficher le lien vers la transaction NFT
		txLink := createExplorerLink(selectedBlockchain.Explorer, "tx", tx.Hash().Hex())
		parsedURL, _ := url.Parse(txLink)
		feedbackLabel.SetText(fmt.Sprintf("Contrat NFT '%s' (%s) déployé !", nftName, nftSymbol))
		link := widget.NewHyperlink("Voir la transaction ici", parsedURL)
		feedbackScroll.Content = container.NewVBox(feedbackLabel, link)
		feedbackScroll.Refresh()
	})

	// Configuration de l'interface utilisateur
	content := container.NewVBox(
		widget.NewLabelWithStyle("Sélection de la Blockchain", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		blockchainSelector,
		widget.NewSeparator(),
		widget.NewLabel("Adresse du Destinataire (Transaction ETH)"),
		recipientEntry,
		widget.NewLabel("Montant en ETH"),
		amountEntry,
		sendButton,
		widget.NewSeparator(),
		widget.NewLabel("Nom du NFT"),
		nameEntry,
		widget.NewLabel("Symbole du NFT"),
		symbolEntry,
		deployNFTButton,
		widget.NewLabel("Clé Privée"),
		privateKeyEntry,
		widget.NewSeparator(),
		widget.NewLabelWithStyle("Résultats", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		feedbackScroll,
	)

	myWindow.SetContent(content)
	myWindow.Resize(fyne.NewSize(400, 600))
	myWindow.CenterOnScreen()
	myWindow.ShowAndRun()
}
