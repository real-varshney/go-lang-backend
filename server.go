package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
)

type GameState struct {
	Username string `json:"username"`
	Score    int    `json:"score"`
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true }, // Replace with origin validation for production
}

var leaderboardSubscribers map[*websocket.Conn]bool
var client *redis.Client

func init() {
	client = redis.NewClient(&redis.Options{
		Addr:     "redis-15342.c264.ap-south-1-1.ec2.cloud.redislabs.com:15342",
		Password: "nYyLbVaJpItDhGWBIgIDBGlMZsRMZhnp",
	})
	_, err := client.Ping(context.Background()).Result()
	if err != nil {
		panic(err)
	}
	leaderboardSubscribers = make(map[*websocket.Conn]bool)
}
func start(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*") // Replace with your frontend's origin
	if r.Method == http.MethodOptions {
		// Handle preflight request
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS") // Allowed methods
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")       // Allowed headers (adjust as needed)
		w.WriteHeader(http.StatusOK)                                         // Set successful status code
		return
	}
	fmt.Fprintf(w, "Server is running")

}


type UserResponse struct {
    Username string
    Score    int
    State    string
}
func addUser(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Access-Control-Allow-Origin", "*") // Replace with your frontend's origin
    if r.Method == http.MethodOptions {
        // Handle preflight request
        w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS") // Allowed methods
        w.Header().Set("Access-Control-Allow-Headers", "Content-Type")       // Allowed headers (adjust as needed)
        w.WriteHeader(http.StatusOK)                                         // Set successful status code
        return
    }
    defer r.Body.Close()

    var user GameState
    err := json.NewDecoder(r.Body).Decode(&user)
    if err != nil {
        http.Error(w, "Invalid request body", http.StatusBadRequest)
        return
    }

    // Check if username already exists
    usernameKey := "user:" + user.Username
    userGameState, err := client.Get(context.Background(), usernameKey).Result()
    if err == nil {
        // Username exists, retrieve score
        var existingUser GameState
        err = json.Unmarshal([]byte(userGameState), &existingUser)
        if err != nil {
            http.Error(w, "Internal server error", http.StatusInternalServerError)
            return
        }
        // Prepare the response object for existing users
        response := UserResponse{
            Username: existingUser.Username,
            Score:    existingUser.Score,
            State:    "already exists",
        }
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(response)
        return
    }

    // If not found, proceed with adding the user
    user.Score = 0 // Set score to 0 for new users
    response := UserResponse{
        Username: user.Username,
        Score:    user.Score,
        State:    "newly added",
    }
    gameStateJSON, err := json.Marshal(user)
    if err != nil {
        http.Error(w, "Internal server error", http.StatusInternalServerError)
        return
    }

    err = client.Set(context.Background(), usernameKey, gameStateJSON, 0).Err()
    if err != nil {
        http.Error(w, "Failed to add user", http.StatusInternalServerError)
        return
    }

    // Send the response object for new users
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(response)
}
func getUser(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*") // Replace with your frontend's origin
	if r.Method == http.MethodOptions {
		// Handle preflight request
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS") // Allowed methods
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		// Allowed headers (adjust as needed)
		w.WriteHeader(http.StatusOK) // Set successful status code
		return
	}
	if r.Method == http.MethodGet {
		username := r.URL.Query().Get("username")
		if username == "" {
			http.Error(w, "Missing username parameter", http.StatusBadRequest)
			return
		}

		gameStateJSON, err := client.Get(context.Background(), "user:"+username).Result()
		if err == redis.Nil {
			http.Error(w, "User not found", http.StatusNotFound)
			return
		} else if err != nil {
			http.Error(w, "Failed to get user", http.StatusInternalServerError)
			return
		}

		var user GameState
		err = json.Unmarshal([]byte(gameStateJSON), &user)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		userJSON, err := json.Marshal(user)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, string(userJSON))

	}
}

func getAllPlayers(client *redis.Client) ([]GameState, error) {
	var players []GameState
	cursor := uint64(0) // Start from the beginning
	matchPattern := ""  // Match all keys with "user:" prefix

	for {
		// Use Scan with three arguments
		keys, cursor, err := client.Scan(context.Background(), cursor, matchPattern, 100).Result()
		if err != nil {
			return nil, err
		}

		for _, key := range keys {
			if strings.HasPrefix(key, "user:") {
				// Ensure proper key format before parsing
				if len(strings.Split(key, ":")) == 2 {
					// Unmarshal directly into GameState struct
					var gameState GameState
					if err := json.Unmarshal([]byte(client.Get(context.Background(), key).Val()), &gameState); err != nil {
						return nil, err
					}
					players = append(players, gameState)
				} else {
					fmt.Println("Invalid key format:", key)
				}
			}
		}

		// Exit loop if cursor is 0 (no more keys to scan)
		if cursor == 0 {
			break
		}
	}

	// Sort players by descending score
	sort.Slice(players, func(i, j int) bool {
		return players[i].Score > players[j].Score
	})

	return players, nil
}

func updateScore(username string, newScore int) error {
	// Update score logic in your application
	gameStateData := GameState{Username: username, Score: newScore}
	gameStateJSON, err := json.Marshal(GameState{Username: username, Score: newScore})
	if err != nil {
		return err
	}

	err = client.Set(context.Background(), "user:"+username, gameStateJSON, 0).Err()
	if err != nil {
		return err
	}

	message := map[string]interface{}{"type": "DataUpdated", "value": gameStateData}
	messageJSON, err := json.Marshal(message)
	if err != nil {
		return err
	}

	for subscriber := range leaderboardSubscribers {
		err = subscriber.WriteMessage(websocket.TextMessage, messageJSON)
		if err != nil {
			// Handle individual subscriber error (optional)
			delete(leaderboardSubscribers, subscriber) // Remove disconnected client
			continue
		}
	}

	return nil
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer ws.Close()

	// Add client to subscriber list
	leaderboardSubscribers[ws] = true
	// Handle messages from the client
	for {
		messageType, message, err := ws.ReadMessage()
		if err != nil {
			fmt.Println(err)
			continue
		}

		if messageType == websocket.TextMessage {
			// Parse message (e.g., subscription request)
			data := map[string]interface{}{}
			err := json.Unmarshal(message, &data)
			if err != nil {
				continue
			}

			if data["type"] == "subscribe" && data["channel"] == "leaderboard_updates" {
				fmt.Println("Client subscribed to leaderboard updates")
				confirmationMessage := map[string]interface{}{
					"type": "subscription_confirmed",
				}
				jsonData, err := json.Marshal(confirmationMessage)
				if err != nil {
					fmt.Println(err)
					return
				}
				ws.WriteMessage(websocket.TextMessage, jsonData)
			}

			if data["type"] == "update" {
				valuemap, ok := data["value"].(map[string]interface{})
				if !ok {
					fmt.Println("Error: 'value' field missing or not a map")
				}
				username, userok := valuemap["username"].(string)
				score, scoreok := valuemap["score"].(float64)

				if scoreok && userok {
					err := updateScore(username, int(score))
					if err != nil {
						fmt.Println("err updating score", err)
					}

					// else {
					// 	confirmationMessage := map[string]interface{}{
					// 		"type": "data added",
					// 	}
					// 	jsonData, err := json.Marshal(confirmationMessage)
					// 	if err != nil {
					// 		fmt.Println(err)
					// 		return
					// 	}
					// 	ws.WriteMessage(websocket.TextMessage, jsonData)

					// }

				} else {
					fmt.Println("err in map", err)
				}

			}
		}
		// delete(leaderboardSubscribers, ws)
		// fmt.Println("Client disconnected", data[name])
	}

	// Remove client from subscriber list on disconnection
}

func getuserdetails(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*") // Replace with your frontend's origin
	if r.Method == http.MethodOptions {
		// Handle preflight request
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS") // Allowed methods
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		// Allowed headers (adjust as needed)
		w.WriteHeader(http.StatusOK) // Set successful status code
		return
	}

	if r.Method == http.MethodGet {
		players, err := getAllPlayers(client) // Leverage existing getAllPlayers function
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Encode players slice as userJSON
		userJSON, err := json.Marshal(players)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, string(userJSON))
	}
}

func main() {
	http.HandleFunc("/", start)
	http.HandleFunc("/add-user", addUser)
	http.HandleFunc("/get-user", getUser)
	http.HandleFunc("/get-user-details", getuserdetails)
	http.HandleFunc("/ws", handleWebSocket)

	fmt.Println("Server listening on port 8080")
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		panic(err)
	}
}
