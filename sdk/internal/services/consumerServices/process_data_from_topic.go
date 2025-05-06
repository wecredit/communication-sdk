package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
	rcs "github.com/wecredit/communication-sdk/sdk/channels/rcs"
	sms "github.com/wecredit/communication-sdk/sdk/channels/sms"
	"github.com/wecredit/communication-sdk/sdk/channels/whatsapp"
	"github.com/wecredit/communication-sdk/sdk/config"
	"github.com/wecredit/communication-sdk/sdk/internal/database"
	"github.com/wecredit/communication-sdk/sdk/internal/queue"
	services "github.com/wecredit/communication-sdk/sdk/internal/services/dbService"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

// ConsumerService consumes messages from an Azure Service Bus subscription
func ConsumerService(count int, topicName, subscriptionName string) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a receiver for the subscription
	receiver, err := queue.Client.NewReceiverForSubscription(topicName, subscriptionName, &azservicebus.ReceiverOptions{
		ReceiveMode: azservicebus.ReceiveModePeekLock,
	})
	if err != nil {
		log.Printf("Failed to create receiver: %v", err)
		return
	}
	defer receiver.Close(ctx)

	batchSize := count // Fetch batch size from config

	// Handle graceful shutdown
	go handleShutdown(cancel)

	for {
		select {
		case <-ctx.Done():
			log.Println("Shutting down consumer service.")
			return
		default:
			// Receive messages in batch
			messages, err := receiver.ReceiveMessages(ctx, batchSize, nil)
			if err != nil {
				log.Printf("Failed to receive messages: %v", err)
				time.Sleep(2 * time.Second) // Avoid tight loop on errors
				continue
			}

			var wg sync.WaitGroup
			sem := make(chan struct{}, 10) // Semaphore for concurrency

			for _, message := range messages {
				if message == nil {
					log.Println("Received a nil message")
					continue
				}

				wg.Add(1)
				sem <- struct{}{} // Acquire a slot

				go func(msg *azservicebus.ReceivedMessage) {
					defer wg.Done()
					defer func() { <-sem }() // Release the slot

					msgCtx, msgCancel := context.WithCancel(ctx)
					defer msgCancel()

					// Start lock renewal in a separate goroutine
					lockRenewalDone := make(chan struct{})
					go renewLock(msgCtx, receiver, msg, lockRenewalDone)

					// Process the message
					if processMessage(msg) {
						// Successfully processed, complete the message
						if err := receiver.CompleteMessage(ctx, msg, nil); err != nil {
							log.Printf("Failed to complete message: %v", err)
						} else {
							log.Println("Message processed and removed from queue.")
						}
					} else {
						// Processing failed, abandon the message
						if err := receiver.AbandonMessage(ctx, msg, nil); err != nil {
							log.Printf("Failed to abandon message: %v", err)
						} else {
							log.Println("Message abandoned and will be retried.")
						}
					}

					// Stop lock renewal
					close(lockRenewalDone)
				}(message)
			}

			wg.Wait()
		}
	}
}

// renewLock continuously renews the message lock until processing is complete or an error occurs
func renewLock(ctx context.Context, receiver *azservicebus.Receiver, msg *azservicebus.ReceivedMessage, done chan struct{}) {
	ticker := time.NewTicker(30 * time.Second) // Set a safe renewal interval
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := receiver.RenewMessageLock(ctx, msg, nil); err != nil {
				log.Printf("Failed to renew lock for message: %v", err)
				return
			}
			log.Println("Lock renewed for message.")
		case <-done:
			// Stop renewing lock when processing is done
			log.Println("Stopping lock renewal for message.")
			return
		case <-ctx.Done():
			log.Println("Context canceled, stopping lock renewal.")
			return
		}
	}
}

// handleShutdown listens for termination signals and shuts down gracefully
func handleShutdown(cancelFunc context.CancelFunc) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigChan
	log.Printf("Received shutdown signal: %v", sig)
	cancelFunc()
}

func processMessage(message *azservicebus.ReceivedMessage) bool {
	var data sdkModels.CommApiRequestBody

	// Unmarshal the message body into the LeadApiRequestData struct
	if err := json.Unmarshal(message.Body, &data); err != nil {
		log.Printf("Failed to unmarshal message body: %v", err)
		return false
	}

	data.Client = strings.ToLower(data.Client)
	data.Channel = strings.ToUpper(data.Channel)
	data.ProcessName = strings.ToUpper(data.ProcessName)
	data.Vendor = strings.ToUpper(data.Vendor)
	// data.Client = strings.ToUpper(data.Client)

	dbMappedData, err := services.MapIntoDbModel(data)
	if err != nil {
		utils.Error(fmt.Errorf("error in mapping data into dbModel: %v", err))
	}

	utils.Debug(fmt.Sprintf("Data: %v", data))

	switch data.Channel {
	case variables.WhatsApp:
		if err := database.InsertData(config.Configs.SdkWhatsappInputTable, database.DBtech, dbMappedData); err != nil {
			utils.Error(fmt.Errorf("error inserting data into table: %v", err))
		}
		data.Vendor = GetVendorByChannel(data.Channel, data.CommId)
		response, err := whatsapp.SendWpByProcess(data)
		if err == nil {
			utils.Debug(fmt.Sprintf("%v", response))
			return true
		} else {
			utils.Error(fmt.Errorf("error in sending whatsapp: %v", err))
			return false
		}
	case variables.RCS:
		if err := database.InsertData(config.Configs.SdkRcsInputTable, database.DBtech, dbMappedData); err != nil {
			utils.Error(fmt.Errorf("error inserting data into table: %v", err))
		}
		data.Vendor = GetVendorByChannel(data.Channel, data.CommId)
		response, err := rcs.SendRcsByProcess(data)
		if err == nil {
			utils.Debug(fmt.Sprintf("%v", response))
			return true
		} else {
			utils.Error(fmt.Errorf("error in sending RCS: %v", err))
			return false
		}

	case variables.SMS:
		// return false
		if err := database.InsertData(config.Configs.SdkSmsInputTable, database.DBtech, dbMappedData); err != nil {
			utils.Error(fmt.Errorf("error inserting data into table: %v", err))
		}
		data.Vendor = GetVendorByChannel(data.Channel, data.CommId)
		response, err := sms.SendSmsByProcess(data)
		if err == nil {
			utils.Debug(fmt.Sprintf("%v", response))
			return true
		} else {
			utils.Error(fmt.Errorf("error in sending SMS: %v", err))
			return false
		}

	default:
		utils.Error(fmt.Errorf("invalid channel: %s", data.Channel))
		return false
	}
}
