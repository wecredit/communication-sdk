package queue

import (
	"fmt"
	"sync"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
)

// Define global variables for the clients
var (
	Client         *azservicebus.Client
	CallbackClient *azservicebus.Client
	mu             sync.Mutex // To ensure thread-safe initialization
)

// Initialize the Azure Service Bus client
func InitClient(connString string, isCallback bool) error {
	mu.Lock()
	defer mu.Unlock()

	var err error

	if isCallback {
		if CallbackClient == nil {
			CallbackClient, err = azservicebus.NewClientFromConnectionString(connString, nil)
			if err != nil {
				return fmt.Errorf("failed to create Callback Azure Service Bus client: %v", err)
			}
			fmt.Println("Callback Azure Bus Service connection successful")
		}
	} else {
		if Client == nil {
			Client, err = azservicebus.NewClientFromConnectionString(connString, nil)
			if err != nil {
				return fmt.Errorf("failed to create Azure Service Bus client: %v", err)
			}
			fmt.Println("Azure Bus Service connection successful")
		}
	}

	return nil
}

// GetClient returns the main client
func GetClient(connString string) *azservicebus.Client {
	if Client == nil {
		if err := InitClient(connString, false); err != nil {
			fmt.Println("Failed to initialize Azure Service Bus client.")
			panic(err)
		}
	}
	return Client
}

// GetCallbackClient returns the callback client
func GetCallbackClient(connString string) *azservicebus.Client {
	if CallbackClient == nil {
		if err := InitClient(connString, true); err != nil {
			fmt.Println("Failed to initialize Callback Azure Service Bus client.")
			panic(err)
		}
	}
	return CallbackClient
}
