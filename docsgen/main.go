package main

import (
	"fmt"
	"os"

	"github.com/filecoin-project/go-fil-markets/retrievalmarket"

	retrievalimpl "github.com/filecoin-project/go-fil-markets/retrievalmarket/impl"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	storageimpl "github.com/filecoin-project/go-fil-markets/storagemarket/impl"
	"github.com/filecoin-project/go-statemachine/fsm"
)

func storageDealStatusCmp(a, b fsm.StateKey) bool {
	aDealStatus := a.(storagemarket.StorageDealStatus)
	bDealStatus := b.(storagemarket.StorageDealStatus)
	return aDealStatus < bDealStatus
}

func retrievalDealStatusCmp(a, b fsm.StateKey) bool {
	aDealStatus := a.(retrievalmarket.DealStatus)
	bDealStatus := b.(retrievalmarket.DealStatus)
	return aDealStatus < bDealStatus
}

func main() {
	file, err := os.Create("./docs/storageclient.mmd")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	err = fsm.GenerateUML(file, fsm.MermaidUML, storageimpl.ClientFSMParameterSpec, storagemarket.DealStates, storagemarket.ClientEvents, []fsm.StateKey{storagemarket.StorageDealUnknown}, false, storageDealStatusCmp)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	err = file.Close()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	file, err = os.Create("./docs/storageprovider.mmd")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	err = fsm.GenerateUML(file, fsm.MermaidUML, storageimpl.ProviderFSMParameterSpec, storagemarket.DealStates, storagemarket.ProviderEvents, []fsm.StateKey{storagemarket.StorageDealUnknown}, false, storageDealStatusCmp)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	err = file.Close()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	file, err = os.Create("./docs/retrievalclient.mmd")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	err = fsm.GenerateUML(file, fsm.MermaidUML, retrievalimpl.ClientFSMParameterSpec, retrievalmarket.DealStatuses, retrievalmarket.ClientEvents, []fsm.StateKey{retrievalmarket.DealStatusNew}, false, retrievalDealStatusCmp)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	err = file.Close()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	file, err = os.Create("./docs/retrievalprovider.mmd")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	err = fsm.GenerateUML(file, fsm.MermaidUML, retrievalimpl.ProviderFSMParameterSpec, retrievalmarket.DealStatuses, retrievalmarket.ProviderEvents, []fsm.StateKey{retrievalmarket.DealStatusNew}, false, retrievalDealStatusCmp)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	err = file.Close()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
