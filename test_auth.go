package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

func main() {
	if len(os.Args) < 4 {
		fmt.Println("Usage: go run test_auth.go <clientId> <clientSecret> <token>")
		os.Exit(1)
	}

	clientID := os.Args[1]
	clientSecret := os.Args[2]
	token := os.Args[3]

	baseURL := "https://api.vpsie.com/apps/v2"

	fmt.Println("Testing VPSie Authentication Methods")
	fmt.Println("=====================================")

	// Test 1: OAuth Client Credentials
	fmt.Println("1. Testing OAuth 2.0 Client Credentials...")
	testOAuth(baseURL, clientID, clientSecret)

	// Test 2: Bearer token from secret
	fmt.Println("\n2. Testing Bearer Token...")
	testBearerToken(baseURL, token)

	// Test 3: API Key in headers
	fmt.Println("\n3. Testing API Key/Secret in Headers...")
	testAPIKeyHeaders(baseURL, clientID, clientSecret)

	// Test 4: Basic Auth with clientId/clientSecret
	fmt.Println("\n4. Testing Basic Auth (clientId:clientSecret)...")
	testBasicAuth(baseURL, clientID, clientSecret)
}

func testOAuth(baseURL, clientID, clientSecret string) {
	reqBody := map[string]string{
		"grant_type":    "client_credentials",
		"client_id":     clientID,
		"client_secret": clientSecret,
	}
	body, _ := json.Marshal(reqBody)

	req, _ := http.NewRequest("POST", baseURL+"/oauth/token", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	printCurl(req, body)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("   Error: %v\n", err)
		return
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	fmt.Printf("   Status: %d\n", resp.StatusCode)
	fmt.Printf("   Response: %s\n", string(respBody))
}

func testBearerToken(baseURL, token string) {
	req, _ := http.NewRequest("GET", baseURL+"/vms", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	printCurl(req, nil)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("   Error: %v\n", err)
		return
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	fmt.Printf("   Status: %d\n", resp.StatusCode)
	fmt.Printf("   Response: %s\n", string(respBody)[:min(200, len(respBody))])
	if resp.StatusCode == 200 {
		fmt.Printf(" ✓ SUCCESS!\n")
	}
}

func testAPIKeyHeaders(baseURL, clientID, clientSecret string) {
	req, _ := http.NewRequest("GET", baseURL+"/vms", nil)
	req.Header.Set("X-API-Key", clientID)
	req.Header.Set("X-API-Secret", clientSecret)
	req.Header.Set("Accept", "application/json")

	printCurl(req, nil)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("   Error: %v\n", err)
		return
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	fmt.Printf("   Status: %d\n", resp.StatusCode)
	fmt.Printf("   Response: %s\n", string(respBody)[:min(200, len(respBody))])
	if resp.StatusCode == 200 {
		fmt.Printf(" ✓ SUCCESS!\n")
	}
}

func testBasicAuth(baseURL, clientID, clientSecret string) {
	req, _ := http.NewRequest("GET", baseURL+"/vms", nil)
	req.SetBasicAuth(clientID, clientSecret)
	req.Header.Set("Accept", "application/json")

	printCurl(req, nil)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("   Error: %v\n", err)
		return
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	fmt.Printf("   Status: %d\n", resp.StatusCode)
	fmt.Printf("   Response: %s\n", string(respBody)[:min(200, len(respBody))])
	if resp.StatusCode == 200 {
		fmt.Printf(" ✓ SUCCESS!\n")
	}
}

func printCurl(req *http.Request, body []byte) {
	fmt.Printf("   curl -X %s '%s'", req.Method, req.URL.String())
	for key, values := range req.Header {
		for _, value := range values {
			// Mask sensitive data
			if key == "Authorization" && len(value) > 20 {
				fmt.Printf(" \\\n     -H '%s: %s...%s'", key, value[:15], value[len(value)-4:])
			} else {
				fmt.Printf(" \\\n     -H '%s: %s'", key, value)
			}
		}
	}
	if len(body) > 0 {
		fmt.Printf(" \\\n     -d '%s'", string(body))
	}
	fmt.Println()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
