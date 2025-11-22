package main

// ziggy_tools.go
// Shared tool definitions for Ziggy AI assistant
// Used by both voice API (OpenAI Realtime) and text API (Azure AI Foundry)
// Ensures consistency across all interaction modes

// GetZiggyTools returns all available tool definitions for Ziggy assistant
// This single source of truth is used by both voice calls and text messages
func GetZiggyTools() []map[string]interface{} {
	return []map[string]interface{}{
		// ============================================
		// TASK MANAGEMENT TOOLS
		// ============================================
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
						"type":        "string",
						"description": "Detailed description of the task (optional)",
					},
					"priority": map[string]interface{}{
						"type":        "string",
						"description": "Priority level: low, medium, high, or urgent",
						"enum":        []string{"low", "medium", "high", "urgent"},
					},
				},
				"required":             []string{"title"},
				
			},
			
		},
		{
			"type":        "function",
			"name":        "list_tasks",
			"description": "List all tasks for the caller. Can optionally filter by status.",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"status": map[string]interface{}{
						"type":        "string",
						"description": "Filter by status: pending, in_progress, completed, or cancelled (optional)",
						"enum":        []string{"pending", "in_progress", "completed", "cancelled"},
					},
				},
				
			},
			
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
						"enum":        []string{"pending", "in_progress", "completed", "cancelled"},
					},
				},
				"required":             []string{"task_id", "status"},
				
			},
			
		},

		// ============================================
		// REMINDER TOOLS
		// ============================================
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
						"type":        "string",
						"description": "Recurrence pattern: 'once' (default, one-time), 'daily', 'weekly', 'monthly', 'yearly'. Only specify if user wants recurring reminder.",
						"enum":        []string{"once", "daily", "weekly", "monthly", "yearly"},
					},
				},
				"required":             []string{"reminder_text", "reminder_time"},
				
			},
			
		},
		{
			"type":        "function",
			"name":        "list_reminders",
			"description": "List all reminders for the caller. Can filter by status (pending, called, completed, cancelled).",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"status": map[string]interface{}{
						"type":        "string",
						"description": "Optional filter by status: 'pending', 'called', 'completed', 'cancelled'. If not provided, shows all reminders.",
						"enum":        []string{"pending", "called", "completed", "cancelled"},
					},
				},
				
			},
			
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
				
			},
			
		},

		// ============================================
		// NOTES TOOLS
		// ============================================
		{
			"type":        "function",
			"name":        "add_note",
			"description": "Create a note for the caller. Use this when they ask to note something, write something down, remember something, or save information.",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"note_content": map[string]interface{}{
						"type":        "string",
						"description": "The content of the note to save",
					},
				},
				"required":             []string{"note_content"},
				
			},
			
		},
		{
			"type":        "function",
			"name":        "list_notes",
			"description": "List all notes for the caller. Shows all saved notes in chronological order.",
			"parameters": map[string]interface{}{
				"type":                 "object",
				"properties":           map[string]interface{}{},
				
			},
			
		},
		{
			"type":        "function",
			"name":        "search_notes",
			"description": "Search through notes for specific keywords or content. Use this when user wants to find specific notes.",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"search_query": map[string]interface{}{
						"type":        "string",
						"description": "Search query to find in notes",
					},
				},
				"required":             []string{"search_query"},
				
			},
			
		},
		{
			"type":        "function",
			"name":        "delete_note",
			"description": "Delete a specific note. Use this when user wants to remove or delete a note.",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"note_id": map[string]interface{}{
						"type":        "string",
						"description": "The ID of the note to delete. Get this from list_notes or search_notes.",
					},
				},
				"required":             []string{"note_id"},
				
			},
			
		},
	}
}
