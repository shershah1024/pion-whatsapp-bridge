package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

// LLMTextHandler handles text message conversations with AI
type LLMTextHandler struct {
	phoneNumber string
	apiKey      string
	endpoint    string
}

// NewLLMTextHandler creates a new LLM text handler
func NewLLMTextHandler(phoneNumber string) *LLMTextHandler {
	// Primary: Azure OpenAI env vars (matching your .env)
	apiKey := os.Getenv("AZURE_OPENAI_API_KEY")
	endpoint := os.Getenv("AZURE_OPENAI_ENDPOINT")

	// Azure AI Foundry - add proper path and API version
	if endpoint != "" && !strings.Contains(endpoint, "/openai/responses") {
		endpoint = endpoint + "/openai/responses?api-version=2025-04-01-preview"
	}

	// Fallback: Azure AI Foundry naming (horizon bot style)
	if apiKey == "" {
		apiKey = os.Getenv("AZURE_API_KEY")
		endpoint = os.Getenv("AZURE_ENDPOINT")
		if endpoint != "" && !strings.Contains(endpoint, "/openai/responses") {
			endpoint = endpoint + "/openai/responses?api-version=2025-04-01-preview"
		}
	}

	// Fallback: Standard OpenAI
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
		endpoint = "https://api.openai.com/v1/chat/completions"
	}

	return &LLMTextHandler{
		phoneNumber: phoneNumber,
		apiKey:      apiKey,
		endpoint:    endpoint,
	}
}

// TextMessage represents a stored text message
type TextMessage struct {
	ID             string `json:"id,omitempty"`
	PhoneNumber    string `json:"phone_number"`
	MessageContent string `json:"message_content"`
	Direction      string `json:"direction"` // "inbound" or "outbound"
	MessageType    string `json:"message_type"`
	Timestamp      string `json:"timestamp,omitempty"`
	CreatedAt      string `json:"created_at,omitempty"`
	MessageID      string `json:"message_id,omitempty"`
	ContactName    string `json:"contact_name,omitempty"`
}

// ChatMessage represents a message in the conversation
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// GetSystemPrompt returns the system prompt for Ziggy (same as voice)
// This uses the same instructions as the voice assistant for consistency
func (h *LLMTextHandler) GetSystemPrompt() string {
	// Detect user's timezone from phone number
	timezone, _ := GetTimezoneFromPhoneNumber(h.phoneNumber)

	// Get current date and time in user's timezone
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		log.Printf("⚠️ Failed to load timezone %s, falling back to UTC: %v", timezone, err)
		loc = time.UTC
	}
	currentTime := time.Now().In(loc)
	currentDateTimeStr := currentTime.Format("Monday, January 2, 2006 at 3:04 PM MST")

	// Same system prompt as voice assistant - consistent experience
	return fmt.Sprintf(`You are Ziggy, a helpful assistant for task management, notes, and reminders via TEXT MESSAGE (WhatsApp).

COMMUNICATION STYLE:
- Keep responses SHORT (1-2 sentences max)
- Use emojis to be friendly 😊
- Be conversational and warm
- Speak ONLY in English

YOUR CAPABILITIES:
1) Tasks - create, list, update tasks (stored permanently)
2) Reminders - set reminders and I'll CALL them back at the specified time
3) Notes - save quick notes and information (use when user says "note that...", "write this down", "remember that...")

CRITICAL RULES - READ CONVERSATION HISTORY CAREFULLY:

1. FOLLOW USER INTENT - Read the conversation context:
   - If user wants to save information → Call add_note tool immediately with the content
   - If user is setting a reminder → Call add_reminder tool immediately when you have text + time
   - If user is creating a task → Call add_task tool immediately when you have a title
   - Don't ask "did you mean task/note/reminder?" if context is clear

2. MINIMIZE FOLLOW-UP QUESTIONS:
   - For notes: Only need note_content (REQUIRED). Save immediately when user provides the information
   - For reminders: Only need reminder_text + reminder_time (both REQUIRED)
   - For tasks: Only need title (REQUIRED). Description and priority are OPTIONAL
   - If user says "no details needed" or "just set it" → Use what you have and call the tool NOW
   - Don't ask for optional fields unless user explicitly wants to provide them

3. BE DECISIVE AND PROACTIVE:
   - If you have the minimum required info → Call the tool immediately
   - Don't keep asking questions when user wants you to "just do it"
   - Example: User says "remind me to call mom tomorrow at 3pm" → Call add_reminder IMMEDIATELY, don't ask for more details

4. MULTI-TURN CONVERSATIONS:
   - When user is clearly continuing a previous request (e.g., you asked "what to remind you about?" and they answered) → Complete the action
   - Don't start new conversations when completing an existing one

CONTEXT:
- Current date and time: %s
- User's timezone: %s
- When setting reminders, convert user's time to format: YYYY-MM-DD HH:MM (24-hour format)

Remember: Be helpful by DOING things quickly, not by asking endless questions!`, currentDateTimeStr, timezone)
}

// SaveMessage saves a text message to Supabase
func (h *LLMTextHandler) SaveMessage(message, direction, messageID, contactName string) error {
	supabaseURL := os.Getenv("SUPABASE_URL")
	supabaseKey := os.Getenv("SUPABASE_ANON_KEY")

	if supabaseURL == "" || supabaseKey == "" {
		log.Println("⚠️ Supabase not configured, message not saved")
		return nil // Don't fail if Supabase isn't configured
	}

	msg := TextMessage{
		PhoneNumber:    h.phoneNumber,
		MessageContent: message,
		Direction:      direction,
		MessageType:    "text",
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
		CreatedAt:      time.Now().UTC().Format(time.RFC3339),
		MessageID:      messageID,
		ContactName:    contactName,
	}

	jsonData, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/rest/v1/ziggy_messages", supabaseURL)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}

	req.Header.Set("apikey", supabaseKey)
	req.Header.Set("Authorization", "Bearer "+supabaseKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Prefer", "return=representation")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusConflict {
		// Duplicate message ID - already saved
		log.Printf("🔁 Message already saved (duplicate ID)")
		return nil
	}

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Supabase error: %s - %s", resp.Status, string(body))
	}

	log.Printf("✅ Saved %s message: %s", direction, message)
	return nil
}

// GetConversationHistory retrieves recent conversation history from Supabase
// Implements the same pattern as horizon bot: first 20 + last 20 messages
func (h *LLMTextHandler) GetConversationHistory() ([]ChatMessage, error) {
	supabaseURL := os.Getenv("SUPABASE_URL")
	supabaseKey := os.Getenv("SUPABASE_ANON_KEY")

	if supabaseURL == "" || supabaseKey == "" {
		log.Println("⚠️ Supabase not configured, no history available")
		return []ChatMessage{}, nil
	}

	// Get total message count
	totalCount, err := h.getMessageCount()
	if err != nil {
		log.Printf("⚠️ Failed to get message count: %v", err)
		return []ChatMessage{}, nil
	}

	if totalCount == 0 {
		return []ChatMessage{}, nil
	}

	var messages []ChatMessage

	if totalCount <= 20 {
		// If 20 or fewer messages, return all in chronological order
		messages, err = h.fetchMessages(totalCount, true)
		if err != nil {
			return []ChatMessage{}, err
		}
		log.Printf("💬 Context: %d messages (all)", totalCount)
	} else {
		// Get first 20 messages (permanent foundation)
		first20, err := h.fetchMessages(20, true)
		if err != nil {
			return []ChatMessage{}, err
		}

		// Get last 20 messages (rolling window)
		last20, err := h.fetchMessages(20, false)
		if err != nil {
			return []ChatMessage{}, err
		}

		// Combine: foundation + recent
		messages = append(first20, last20...)
		log.Printf("💬 Context: 40 messages (20 foundation + 20 recent) from total %d", totalCount)
	}

	return messages, nil
}

// getMessageCount gets the total message count for this phone number
func (h *LLMTextHandler) getMessageCount() (int, error) {
	supabaseURL := os.Getenv("SUPABASE_URL")
	supabaseKey := os.Getenv("SUPABASE_ANON_KEY")

	url := fmt.Sprintf("%s/rest/v1/ziggy_messages?phone_number=eq.%s&select=id",
		supabaseURL, h.phoneNumber)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, err
	}

	req.Header.Set("apikey", supabaseKey)
	req.Header.Set("Authorization", "Bearer "+supabaseKey)
	req.Header.Set("Prefer", "count=exact")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("failed to get count: %s", resp.Status)
	}

	// Count is in Content-Range header: "0-9/10" means 10 total
	contentRange := resp.Header.Get("Content-Range")
	var total int
	fmt.Sscanf(contentRange, "%*d-%*d/%d", &total)

	return total, nil
}

// fetchMessages fetches messages in specified order
func (h *LLMTextHandler) fetchMessages(limit int, ascending bool) ([]ChatMessage, error) {
	supabaseURL := os.Getenv("SUPABASE_URL")
	supabaseKey := os.Getenv("SUPABASE_ANON_KEY")

	order := "timestamp.asc"
	if !ascending {
		order = "timestamp.desc"
	}

	url := fmt.Sprintf("%s/rest/v1/ziggy_messages?phone_number=eq.%s&order=%s&limit=%d&select=message_content,direction,timestamp",
		supabaseURL, h.phoneNumber, order, limit)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("apikey", supabaseKey)
	req.Header.Set("Authorization", "Bearer "+supabaseKey)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to fetch messages: %s - %s", resp.Status, string(body))
	}

	var dbMessages []struct {
		MessageContent string `json:"message_content"`
		Direction      string `json:"direction"`
		Timestamp      string `json:"timestamp"`
	}

	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &dbMessages); err != nil {
		return nil, err
	}

	// Convert to chat messages
	var messages []ChatMessage
	for _, msg := range dbMessages {
		role := "user"
		if msg.Direction == "outbound" {
			role = "assistant"
		}
		messages = append(messages, ChatMessage{
			Role:    role,
			Content: msg.MessageContent,
		})
	}

	// If we fetched in descending order, reverse to chronological
	if !ascending {
		for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
			messages[i], messages[j] = messages[j], messages[i]
		}
	}

	return messages, nil
}

// GetTools returns the tool definitions for Ziggy
// Note: Cannot use GetZiggyTools() because Azure AI Foundry with strict:true requires
// different schema format (nullable types) than OpenAI Realtime API
func (h *LLMTextHandler) GetTools() []map[string]interface{} {
	return h.GetToolsOLD() // Use Azure-compatible format
}

// GetToolsOLD is the old hardcoded version - kept for reference, can be deleted
func (h *LLMTextHandler) GetToolsOLD() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"type":        "function",
			"name":        "add_task",
			"description": "Create a new task for the caller. Use this when they ask to add, create, or remember a task or todo item.",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"title": map[string]interface{}{
						"type":        "string",
						"description": "Brief title of the task",
					},
					"description": map[string]interface{}{
						"type":        []string{"string", "null"},
						"description": "Detailed description of the task (optional)",
					},
					"priority": map[string]interface{}{
						"type":        []string{"string", "null"},
						"description": "Priority level: low, medium, high, or urgent",
						"enum":        []interface{}{"low", "medium", "high", "urgent", nil},
					},
				},
				"required":             []string{"title", "description", "priority"},
				"additionalProperties": false,
			},
			"strict": true,
		},
		{
			"type":        "function",
			"name":        "list_tasks",
			"description": "List all tasks for the caller. Can optionally filter by status.",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"status": map[string]interface{}{
						"type":        []string{"string", "null"},
						"description": "Filter by status: pending, in_progress, completed, or cancelled (optional)",
						"enum":        []interface{}{"pending", "in_progress", "completed", "cancelled", nil},
					},
				},
				"required":             []string{"status"},
				"additionalProperties": false,
			},
			"strict": true,
		},
		{
			"type":        "function",
			"name":        "update_task_status",
			"description": "Update the status of a task. Use when user wants to mark task as done, complete, in progress, etc.",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"task_id": map[string]interface{}{
						"type":        "string",
						"description": "The ID of the task to update",
					},
					"status": map[string]interface{}{
						"type":        "string",
						"description": "New status: pending, in_progress, completed, or cancelled",
						"enum":        []interface{}{"pending", "in_progress", "completed", "cancelled"},
					},
				},
				"required":             []string{"task_id", "status"},
				"additionalProperties": false,
			},
			"strict": true,
		},
		{
			"type":        "function",
			"name":        "add_reminder",
			"description": "Set a reminder for the caller. Supports one-time and recurring reminders. When the reminder time comes, Ziggy will call them back.",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"reminder_text": map[string]interface{}{
						"type":        "string",
						"description": "What to remind the user about",
					},
					"reminder_time": map[string]interface{}{
						"type":        "string",
						"description": "When to send the reminder in local timezone using format YYYY-MM-DD HH:MM (e.g., 2025-11-09 14:30 for 2:30 PM). Use 24-hour format. Ask the user for the exact date and time if not provided.",
					},
					"recurrence": map[string]interface{}{
						"type":        []string{"string", "null"},
						"description": "Recurrence pattern: 'once' (default, one-time), 'daily', 'weekly', 'monthly', 'yearly'. Only specify if user wants recurring reminder.",
						"enum":        []interface{}{"once", "daily", "weekly", "monthly", "yearly", nil},
					},
				},
				"required":             []string{"reminder_text", "reminder_time", "recurrence"},
				"additionalProperties": false,
			},
			"strict": true,
		},
		{
			"type":        "function",
			"name":        "list_reminders",
			"description": "List all reminders for the caller. Can filter by status (pending, called, completed, cancelled).",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"status": map[string]interface{}{
						"type":        []string{"string", "null"},
						"description": "Optional filter by status: 'pending', 'called', 'completed', 'cancelled'. If not provided, shows all reminders.",
						"enum":        []interface{}{"pending", "called", "completed", "cancelled", nil},
					},
				},
				"required":             []string{"status"},
				"additionalProperties": false,
			},
			"strict": true,
		},
		{
			"type":        "function",
			"name":        "cancel_reminder",
			"description": "Cancel a reminder. Use this when user wants to stop or delete a reminder.",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"reminder_id": map[string]interface{}{
						"type":        "string",
						"description": "The ID of the reminder to cancel. Get this from list_reminders.",
					},
				},
				"required":             []string{"reminder_id"},
				"additionalProperties": false,
			},
			"strict": true,
		},
		{
			"type":        "function",
			"name":        "add_note",
			"description": "Create a note for the caller. Use this when they ask to note something, write something down, remember something, or save information. Examples: 'Note that...', 'Write this down', 'Remember that...'",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"note_content": map[string]interface{}{
						"type":        "string",
						"description": "The content of the note to save",
					},
				},
				"required":             []string{"note_content"},
				"additionalProperties": false,
			},
			"strict": true,
		},
		{
			"type":        "function",
			"name":        "list_notes",
			"description": "List all notes for the caller.",
			"parameters": map[string]interface{}{
				"type":                 "object",
				"properties":           map[string]interface{}{},
				"required":             []string{},
				"additionalProperties": false,
			},
			"strict": true,
		},
		{
			"type":        "function",
			"name":        "search_notes",
			"description": "Search through the caller's notes using keywords. Use this when they want to find specific notes.",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"search_query": map[string]interface{}{
						"type":        "string",
						"description": "Keywords to search for in notes",
					},
				},
				"required":             []string{"search_query"},
				"additionalProperties": false,
			},
			"strict": true,
		},
		{
			"type":        "function",
			"name":        "delete_note",
			"description": "Delete a note. Use this when user wants to remove or delete a note.",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"note_id": map[string]interface{}{
						"type":        "string",
						"description": "The ID of the note to delete. Get this from list_notes.",
					},
				},
				"required":             []string{"note_id"},
				"additionalProperties": false,
			},
			"strict": true,
		},
	}
}

// GetAIResponse gets an AI response from Azure OpenAI or OpenAI
func (h *LLMTextHandler) GetAIResponse(userMessage string) (string, error) {
	if h.apiKey == "" || h.endpoint == "" {
		return "I'm sorry, I'm not configured properly. Please check the API settings.", fmt.Errorf("API not configured")
	}

	// Get conversation history
	history, err := h.GetConversationHistory()
	if err != nil {
		log.Printf("⚠️ Failed to get history: %v", err)
		history = []ChatMessage{} // Continue with empty history
	}

	// Build messages array (this will be the input)
	messages := []interface{}{
		map[string]interface{}{
			"role":    "system",
			"content": h.GetSystemPrompt(),
		},
	}

	// Add conversation history
	for _, msg := range history {
		messages = append(messages, map[string]interface{}{
			"role":    msg.Role,
			"content": msg.Content,
		})
	}

	// Add current message
	messages = append(messages, map[string]interface{}{
		"role":    "user",
		"content": userMessage,
	})

	// Log conversation context for debugging
	log.Printf("💭 Sending to LLM: %d history messages + current message", len(history))
	if len(history) > 0 {
		log.Printf("📝 Last 3 messages in history:")
		start := len(history) - 3
		if start < 0 {
			start = 0
		}
		for i := start; i < len(history); i++ {
			log.Printf("   [%s]: %s", history[i].Role, history[i].Content)
		}
	}
	log.Printf("   [user]: %s", userMessage)

	// Make initial request with tools
	return h.makeRequestWithTools(messages)
}

// makeRequestWithTools handles the full tool calling flow
func (h *LLMTextHandler) makeRequestWithTools(input []interface{}) (string, error) {
	// Prepare request with tools
	requestBody := map[string]interface{}{
		"input":             input,
		"max_output_tokens": 3000,
		"model":             "gpt-5-mini",
		"tools":             h.GetTools(),
		"tool_choice":       "auto",
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return "", err
	}

	log.Printf("🤖 Calling LLM API: %s (model: gpt-5-mini, max_tokens: 3000)", h.endpoint)
	// Only log full request body if there's an error (too verbose otherwise)

	req, err := http.NewRequest("POST", h.endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}

	// Azure AI Foundry uses Bearer token
	req.Header.Set("Authorization", "Bearer "+h.apiKey)
	req.Header.Set("Content-Type", "application/json")

	log.Printf("🔑 Auth header: Bearer %s...", h.apiKey[:20])

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("❌ LLM API request failed: %v", err)
		return "I'm having trouble thinking right now. Can you try again?", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		log.Printf("❌ LLM API error: Status=%s, Body=%s", resp.Status, string(body))
		log.Printf("📤 Request that failed: %s", string(jsonData))
		return "I'm having trouble thinking right now. Can you try again? 🤔", fmt.Errorf("API error: %s", resp.Status)
	}

	log.Printf("✅ LLM API response received successfully")

	// Parse response to check for function calls
	var result struct {
		Output []map[string]interface{} `json:"output"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		log.Printf("❌ Failed to parse JSON response: %v", err)
		log.Printf("📥 Raw response: %s", string(body))
		return "", err
	}

	// Check if there are function calls
	var hasFunctionCalls bool
	for _, output := range result.Output {
		if outputType, ok := output["type"].(string); ok && outputType == "function_call" {
			hasFunctionCalls = true
			break
		}
	}

	// If there are function calls, handle them
	if hasFunctionCalls {
		return h.handleFunctionCalls(input, result.Output)
	}

	// Otherwise extract text response
	var aiResponse string
	for _, output := range result.Output {
		if outputType, ok := output["type"].(string); ok && outputType == "message" {
			if content, ok := output["content"].([]interface{}); ok {
				for _, c := range content {
					if contentMap, ok := c.(map[string]interface{}); ok {
						if contentType, ok := contentMap["type"].(string); ok && contentType == "output_text" {
							if text, ok := contentMap["text"].(string); ok && text != "" {
								aiResponse = text
								break
							}
						}
					}
				}
			}
			if aiResponse != "" {
				break
			}
		}
	}

	if aiResponse == "" {
		log.Printf("❌ No text found in response output")
		return "", fmt.Errorf("no response from LLM")
	}

	log.Printf("🤖 AI response: %s", aiResponse)
	return aiResponse, nil
}

// handleFunctionCalls processes function calls and makes a second request
func (h *LLMTextHandler) handleFunctionCalls(input []interface{}, output []map[string]interface{}) (string, error) {
	log.Printf("🔧 Processing function calls...")

	// Add all output items to input (including reasoning)
	for _, item := range output {
		input = append(input, item)
	}

	// Process each function call
	for _, item := range output {
		if itemType, ok := item["type"].(string); ok && itemType == "function_call" {
			callID, _ := item["call_id"].(string)
			name, _ := item["name"].(string)
			arguments, _ := item["arguments"].(string)

			log.Printf("📞 Function call: %s(%s)", name, arguments)

			// Execute the function
			result := h.executeFunction(name, arguments)

			// Add function call output to input
			input = append(input, map[string]interface{}{
				"type":    "function_call_output",
				"call_id": callID,
				"output":  result,
			})

			log.Printf("✅ Function %s executed: %s", name, result)
		}
	}

	// Make second request with function results
	log.Printf("🔄 Making second request with function results...")
	return h.makeRequestWithTools(input)
}

// executeFunction executes a function and returns the result as JSON string
func (h *LLMTextHandler) executeFunction(name, arguments string) string {
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return `{"status": "error", "message": "Invalid arguments"}`
	}

	switch name {
	case "add_task":
		title, _ := args["title"].(string)
		description, _ := args["description"].(string)
		priority, _ := args["priority"].(string)
		if priority == "" {
			priority = "medium"
		}

		task, err := AddTask(title, description, priority, h.phoneNumber)
		if err != nil {
			return fmt.Sprintf(`{"status": "error", "message": "%s"}`, err.Error())
		}

		result, _ := json.Marshal(map[string]interface{}{
			"status":  "success",
			"message": fmt.Sprintf("Task '%s' created successfully", title),
			"task_id": task.ID,
			"title":   task.Title,
		})
		return string(result)

	case "list_tasks":
		status, _ := args["status"].(string)

		tasks, err := ListTasks(h.phoneNumber, status)
		if err != nil {
			return fmt.Sprintf(`{"status": "error", "message": "%s"}`, err.Error())
		}

		taskList := make([]map[string]interface{}, len(tasks))
		for i, task := range tasks {
			taskList[i] = map[string]interface{}{
				"id":          task.ID,
				"title":       task.Title,
				"description": task.Description,
				"status":      task.Status,
				"priority":    task.Priority,
			}
		}

		result, _ := json.Marshal(map[string]interface{}{
			"status": "success",
			"count":  len(tasks),
			"tasks":  taskList,
		})
		return string(result)

	case "update_task_status":
		taskID, _ := args["task_id"].(string)
		newStatus, _ := args["status"].(string)

		err := UpdateTaskStatus(taskID, newStatus)
		if err != nil {
			return fmt.Sprintf(`{"status": "error", "message": "%s"}`, err.Error())
		}

		result, _ := json.Marshal(map[string]interface{}{
			"status":  "success",
			"message": fmt.Sprintf("Task updated to %s", newStatus),
			"task_id": taskID,
		})
		return string(result)

	case "add_reminder":
		reminderText, _ := args["reminder_text"].(string)
		reminderTime, _ := args["reminder_time"].(string)
		recurrence, _ := args["recurrence"].(string)
		if recurrence == "" {
			recurrence = "once"
		}

		reminder, err := AddReminder(reminderText, reminderTime, h.phoneNumber, recurrence)
		if err != nil {
			return fmt.Sprintf(`{"status": "error", "message": "%s"}`, err.Error())
		}

		var message string
		if recurrence == "once" {
			message = fmt.Sprintf("Reminder set for %s. I'll call you back at that time.", reminderTime)
		} else {
			message = fmt.Sprintf("%s reminder set for %s. I'll call you back %s.", recurrence, reminderTime, recurrence)
		}

		result, _ := json.Marshal(map[string]interface{}{
			"status":        "success",
			"message":       message,
			"reminder_id":   reminder.ID,
			"reminder_text": reminder.ReminderText,
			"reminder_time": reminder.ReminderTime,
			"recurrence":    reminder.RecurrencePattern,
		})
		return string(result)

	case "list_reminders":
		status, _ := args["status"].(string)

		reminders, err := ListReminders(h.phoneNumber, status)
		if err != nil {
			return fmt.Sprintf(`{"status": "error", "message": "%s"}`, err.Error())
		}

		var reminderList []map[string]interface{}
		for _, r := range reminders {
			reminderList = append(reminderList, map[string]interface{}{
				"id":         r.ID,
				"text":       r.ReminderText,
				"time":       r.ReminderTime,
				"recurrence": r.RecurrencePattern,
				"status":     r.Status,
			})
		}

		result, _ := json.Marshal(map[string]interface{}{
			"status":    "success",
			"count":     len(reminders),
			"reminders": reminderList,
		})
		return string(result)

	case "cancel_reminder":
		reminderID, _ := args["reminder_id"].(string)

		err := CancelReminder(reminderID)
		if err != nil {
			return fmt.Sprintf(`{"status": "error", "message": "%s"}`, err.Error())
		}

		result, _ := json.Marshal(map[string]interface{}{
			"status":      "success",
			"message":     "Reminder cancelled successfully",
			"reminder_id": reminderID,
		})
		return string(result)

	case "add_note":
		noteContent, _ := args["note_content"].(string)

		note, err := AddNote(noteContent, h.phoneNumber)
		if err != nil {
			return fmt.Sprintf(`{"status": "error", "message": "%s"}`, err.Error())
		}

		result, _ := json.Marshal(map[string]interface{}{
			"status":  "success",
			"message": "Note saved successfully",
			"note_id": note.ID,
			"content": note.NoteContent,
		})
		return string(result)

	case "list_notes":
		notes, err := ListNotes(h.phoneNumber, 0)
		if err != nil {
			return fmt.Sprintf(`{"status": "error", "message": "%s"}`, err.Error())
		}

		noteList := make([]map[string]interface{}, len(notes))
		for i, note := range notes {
			noteList[i] = map[string]interface{}{
				"id":      note.ID,
				"content": note.NoteContent,
			}
		}

		result, _ := json.Marshal(map[string]interface{}{
			"status": "success",
			"count":  len(notes),
			"notes":  noteList,
		})
		return string(result)

	case "search_notes":
		searchQuery, _ := args["search_query"].(string)

		notes, err := SearchNotes(h.phoneNumber, searchQuery)
		if err != nil {
			return fmt.Sprintf(`{"status": "error", "message": "%s"}`, err.Error())
		}

		noteList := make([]map[string]interface{}, len(notes))
		for i, note := range notes {
			noteList[i] = map[string]interface{}{
				"id":      note.ID,
				"content": note.NoteContent,
			}
		}

		result, _ := json.Marshal(map[string]interface{}{
			"status": "success",
			"count":  len(notes),
			"query":  searchQuery,
			"notes":  noteList,
		})
		return string(result)

	case "delete_note":
		noteID, _ := args["note_id"].(string)

		err := DeleteNote(noteID)
		if err != nil {
			return fmt.Sprintf(`{"status": "error", "message": "%s"}`, err.Error())
		}

		result, _ := json.Marshal(map[string]interface{}{
			"status":  "success",
			"message": "Note deleted successfully",
			"note_id": noteID,
		})
		return string(result)

	default:
		return fmt.Sprintf(`{"status": "error", "message": "Unknown function: %s"}`, name)
	}
}
