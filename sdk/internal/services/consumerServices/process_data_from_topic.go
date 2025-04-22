package services

import (
	"context"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"dev.azure.com/wctec/communication-engine/sdk/internal/queue"
	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
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
					// if processMessage(msg) {
					// 	// Successfully processed, complete the message
					// 	if err := receiver.CompleteMessage(ctx, msg, nil); err != nil {
					// 		log.Printf("Failed to complete message: %v", err)
					// 	} else {
					// 		log.Println("Message processed and removed from queue.")
					// 	}
					// } else {
					// 	// Processing failed, abandon the message
					// 	if err := receiver.AbandonMessage(ctx, msg, nil); err != nil {
					// 		log.Printf("Failed to abandon message: %v", err)
					// 	} else {
					// 		log.Println("Message abandoned and will be retried.")
					// 	}
					// }

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
