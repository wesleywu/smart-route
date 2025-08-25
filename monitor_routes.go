package main

import (
	"fmt"
	"os"
	"time"

	"github.com/wesleywu/update-routes-native/internal/routing"
)

func main() {
	fmt.Println("=== Route Monitor (Press Ctrl+C to stop) ===")
	
	// Create route manager
	rm, err := routing.NewRouteManager(10, 3)
	if err != nil {
		fmt.Printf("Failed to create route manager: %v\n", err)
		os.Exit(1)
	}
	defer rm.Close()

	dnsIPs := []string{"114.114.114.114", "114.114.115.115", "223.5.5.5", "223.6.6.6"}
	
	for i := 0; i < 10; i++ {
		fmt.Printf("\n--- Check %d at %s ---\n", i+1, time.Now().Format("15:04:05"))
		
		// Get routes
		routes, err := rm.ListRoutes()
		if err != nil {
			fmt.Printf("Error listing routes: %v\n", err)
			continue
		}
		
		fmt.Printf("Total routes: %d\n", len(routes))
		
		// Count DNS routes
		dnsCount := 0
		for _, route := range routes {
			routeIPStr := route.Network.IP.String()
			for _, dnsIP := range dnsIPs {
				if routeIPStr == dnsIP {
					fmt.Printf("  DNS route: %s -> %s (%s)\n", 
						route.Network.String(), route.Gateway.String(), route.Interface)
					dnsCount++
					break
				}
			}
		}
		
		fmt.Printf("DNS routes found: %d\n", dnsCount)
		
		if i < 9 {
			time.Sleep(10 * time.Second)
		}
	}
	
	fmt.Println("\nMonitoring completed.")
}