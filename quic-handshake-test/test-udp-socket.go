package main

import (
	"fmt"
	"net"
	"time"
)

func main() {
	serverAddr := "100.71.189.42:9898"
	
	fmt.Printf("Testing basic UDP socket creation...\n")
	
	// Test 1: Resolve address
	fmt.Printf("1. Resolving address...\n")
	addr, err := net.ResolveUDPAddr("udp", serverAddr)
	if err != nil {
		panic(fmt.Sprintf("Resolve failed: %v", err))
	}
	fmt.Printf("   Resolved: %s\n", addr)
	
	// Test 2: Create UDP connection
	fmt.Printf("2. Creating UDP connection...\n")
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		panic(fmt.Sprintf("DialUDP failed: %v", err))
	}
	defer conn.Close()
	fmt.Printf("   UDP connection created: %s -> %s\n", conn.LocalAddr(), conn.RemoteAddr())
	
	// Test 3: Send a test packet
	fmt.Printf("3. Sending test packet...\n")
	testData := []byte("test")
	n, err := conn.Write(testData)
	if err != nil {
		panic(fmt.Sprintf("Write failed: %v", err))
	}
	fmt.Printf("   Sent %d bytes\n", n)
	
	// Test 4: Try to read (with timeout)
	fmt.Printf("4. Setting read deadline...\n")
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 1024)
	n, err = conn.Read(buf)
	if err != nil {
		fmt.Printf("   Read timeout/error (expected): %v\n", err)
	} else {
		fmt.Printf("   Received %d bytes: %s\n", n, string(buf[:n]))
	}
	
	fmt.Println("Basic UDP socket test completed successfully!")
}

