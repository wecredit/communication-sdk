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
func InitClient(connString string, isCallback bool) (*azservicebus.Client, error) {
	mu.Lock()
	defer mu.Unlock()
	if isCallback {
		if CallbackClient == nil {
			callbackClient, err := azservicebus.NewClientFromConnectionString(connString, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to create Callback Azure Service Bus client: %v", err)
			}
			fmt.Println("Callback Azure Bus Service connection successful")
			return callbackClient, nil
		}
	} else {
		if Client == nil {
			client, err := azservicebus.NewClientFromConnectionString(connString, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to create Azure Service Bus client: %v", err)
			}
			fmt.Println("Azure Bus Service connection successful")
			return client, nil
		}
	}

	return nil, nil
}

// GetClient returns the main client
func GetClient(connString string) *azservicebus.Client {
	var err error
	if Client == nil {
		if Client, err = InitClient(connString, false); err != nil {
			fmt.Println("Failed to initialize Azure Service Bus client.")
			panic(err)
		}
	}
	return Client
}

// GetCallbackClient returns the callback client
func GetCallbackClient(connString string) *azservicebus.Client {
	var err error
	if CallbackClient == nil {
		if CallbackClient, err = InitClient(connString, true); err != nil {
			fmt.Println("Failed to initialize Callback Azure Service Bus client.")
			panic(err)
		}
	}
	return CallbackClient
}

func GetSdkQueueClient(connString string) *azservicebus.Client {
	var err error
	var client *azservicebus.Client
	if client, err = InitClient(connString, true); err != nil {
		fmt.Println("Failed to initialize Azure Service Bus client.")
		panic(err)
	}
	return client
}
