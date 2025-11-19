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

	"github.com/nyaruka/phonenumbers"
)

// Supabase client helper functions for task management

type ZiggyTask struct {
	ID          string    `json:"id,omitempty"`
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`
	Status      string    `json:"status,omitempty"`
	Priority    string    `json:"priority,omitempty"`
	DueDate     string    `json:"due_date,omitempty"`
	CreatedAt   string    `json:"created_at,omitempty"`
	UpdatedAt   string    `json:"updated_at,omitempty"`
	CreatedBy   string    `json:"created_by,omitempty"`
	PhoneNumber string    `json:"phone_number,omitempty"`
}

// AddTask creates a new task in Supabase
func AddTask(title, description, priority, phoneNumber string) (*ZiggyTask, error) {
	supabaseURL := os.Getenv("SUPABASE_URL")
	supabaseKey := os.Getenv("SUPABASE_ANON_KEY")

	if supabaseURL == "" || supabaseKey == "" {
		return nil, fmt.Errorf("Supabase credentials not configured")
	}

	task := ZiggyTask{
		Title:       title,
		Description: description,
		Priority:    priority,
		PhoneNumber: phoneNumber,
		Status:      "pending",
		CreatedBy:   "whatsapp_voice_agent",
	}

	jsonData, err := json.Marshal(task)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/rest/v1/ziggy_tasks", supabaseURL)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}

	req.Header.Set("apikey", supabaseKey)
	req.Header.Set("Authorization", "Bearer "+supabaseKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Prefer", "return=representation")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Supabase error: %s - %s", resp.Status, string(body))
	}

	var tasks []ZiggyTask
	if err := json.Unmarshal(body, &tasks); err != nil {
		return nil, err
	}

	if len(tasks) == 0 {
		return nil, fmt.Errorf("no task returned from Supabase")
	}

	log.Printf("✅ Task created in Supabase: %s (ID: %s)", tasks[0].Title, tasks[0].ID)
	return &tasks[0], nil
}

// ListTasks retrieves tasks for a phone number
func ListTasks(phoneNumber string, status string) ([]ZiggyTask, error) {
	supabaseURL := os.Getenv("SUPABASE_URL")
	supabaseKey := os.Getenv("SUPABASE_ANON_KEY")

	if supabaseURL == "" || supabaseKey == "" {
		return nil, fmt.Errorf("Supabase credentials not configured")
	}

	url := fmt.Sprintf("%s/rest/v1/ziggy_tasks?phone_number=eq.%s&order=created_at.desc",
		supabaseURL, phoneNumber)

	if status != "" {
		url += fmt.Sprintf("&status=eq.%s", status)
	}

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

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Supabase error: %s - %s", resp.Status, string(body))
	}

	var tasks []ZiggyTask
	if err := json.Unmarshal(body, &tasks); err != nil {
		return nil, err
	}

	log.Printf("📋 Retrieved %d tasks from Supabase for %s", len(tasks), phoneNumber)
	return tasks, nil
}

// UpdateTaskStatus updates the status of a task
func UpdateTaskStatus(taskID, status string) error {
	supabaseURL := os.Getenv("SUPABASE_URL")
	supabaseKey := os.Getenv("SUPABASE_ANON_KEY")

	if supabaseURL == "" || supabaseKey == "" {
		return fmt.Errorf("Supabase credentials not configured")
	}

	update := map[string]string{
		"status": status,
	}

	jsonData, err := json.Marshal(update)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/rest/v1/ziggy_tasks?id=eq.%s", supabaseURL, taskID)
	req, err := http.NewRequest("PATCH", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}

	req.Header.Set("apikey", supabaseKey)
	req.Header.Set("Authorization", "Bearer "+supabaseKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("Supabase error: %s - %s", resp.Status, string(body))
	}

	log.Printf("✅ Task %s updated to status: %s", taskID, status)
	return nil
}

// ZiggyReminder represents a reminder
type ZiggyReminder struct {
	ID                string `json:"id,omitempty"`
	PhoneNumber       string `json:"phone_number"`
	ReminderText      string `json:"reminder_text"`
	ReminderTime      string `json:"reminder_time"`
	RecurrencePattern string `json:"recurrence_pattern,omitempty"` // null/"once", "daily", "weekly", "monthly", "yearly"
	Status            string `json:"status,omitempty"`
	CreatedAt         string `json:"created_at,omitempty"`
	UpdatedAt         string `json:"updated_at,omitempty"`
	CallID            string `json:"call_id,omitempty"`
	Attempts          int    `json:"attempts,omitempty"`
}

// GetTimezoneFromPhoneNumber detects the timezone based on the phone number's country code
// Returns the IANA timezone name (e.g., "Asia/Kolkata", "America/New_York")
func GetTimezoneFromPhoneNumber(phoneNumber string) (string, error) {
	// Add + prefix if not present (required by phonenumbers library)
	if !strings.HasPrefix(phoneNumber, "+") {
		phoneNumber = "+" + phoneNumber
	}

	// Parse the phone number
	num, err := phonenumbers.Parse(phoneNumber, "")
	if err != nil {
		log.Printf("⚠️ Failed to parse phone number %s, defaulting to Asia/Kolkata: %v", phoneNumber, err)
		return "Asia/Kolkata", nil // Default to IST for backwards compatibility
	}

	// Get country code
	regionCode := phonenumbers.GetRegionCodeForNumber(num)

	// Map country codes to primary timezone
	// For countries with multiple timezones, we use the most populous/capital city timezone
	countryToTimezone := map[string]string{
		// Asia
		"IN": "Asia/Kolkata",        // India - IST (UTC+5:30)
		"CN": "Asia/Shanghai",        // China - CST (UTC+8)
		"JP": "Asia/Tokyo",           // Japan - JST (UTC+9)
		"SG": "Asia/Singapore",       // Singapore - SGT (UTC+8)
		"HK": "Asia/Hong_Kong",       // Hong Kong - HKT (UTC+8)
		"TH": "Asia/Bangkok",         // Thailand - ICT (UTC+7)
		"MY": "Asia/Kuala_Lumpur",    // Malaysia - MYT (UTC+8)
		"ID": "Asia/Jakarta",         // Indonesia - WIB (UTC+7, most populous)
		"PH": "Asia/Manila",          // Philippines - PHT (UTC+8)
		"VN": "Asia/Ho_Chi_Minh",     // Vietnam - ICT (UTC+7)
		"KR": "Asia/Seoul",           // South Korea - KST (UTC+9)
		"PK": "Asia/Karachi",         // Pakistan - PKT (UTC+5)
		"BD": "Asia/Dhaka",           // Bangladesh - BST (UTC+6)
		"LK": "Asia/Colombo",         // Sri Lanka - IST (UTC+5:30)
		"AE": "Asia/Dubai",           // UAE - GST (UTC+4)
		"SA": "Asia/Riyadh",          // Saudi Arabia - AST (UTC+3)
		"IL": "Asia/Jerusalem",       // Israel - IST (UTC+2/3)

		// Americas
		"US": "America/New_York",     // USA - EST (most populous eastern timezone)
		"CA": "America/Toronto",      // Canada - EST (most populous)
		"MX": "America/Mexico_City",  // Mexico - CST (capital)
		"BR": "America/Sao_Paulo",    // Brazil - BRT (most populous)
		"AR": "America/Argentina/Buenos_Aires", // Argentina - ART (UTC-3)
		"CL": "America/Santiago",     // Chile - CLT (UTC-3/4)
		"CO": "America/Bogota",       // Colombia - COT (UTC-5)
		"PE": "America/Lima",         // Peru - PET (UTC-5)

		// Europe
		"GB": "Europe/London",        // UK - GMT/BST (UTC+0/1)
		"GG": "Europe/London",        // Guernsey - GMT/BST (UTC+0/1)
		"JE": "Europe/London",        // Jersey - GMT/BST (UTC+0/1)
		"IM": "Europe/London",        // Isle of Man - GMT/BST (UTC+0/1)
		"IE": "Europe/Dublin",        // Ireland - GMT/IST (UTC+0/1)
		"PT": "Europe/Lisbon",        // Portugal - WET/WEST (UTC+0/1)
		"DE": "Europe/Berlin",        // Germany - CET (UTC+1/2)
		"FR": "Europe/Paris",         // France - CET (UTC+1/2)
		"IT": "Europe/Rome",          // Italy - CET (UTC+1/2)
		"ES": "Europe/Madrid",        // Spain - CET (UTC+1/2)
		"NL": "Europe/Amsterdam",     // Netherlands - CET (UTC+1/2)
		"BE": "Europe/Brussels",      // Belgium - CET (UTC+1/2)
		"LU": "Europe/Luxembourg",    // Luxembourg - CET (UTC+1/2)
		"CH": "Europe/Zurich",        // Switzerland - CET (UTC+1/2)
		"AT": "Europe/Vienna",        // Austria - CET (UTC+1/2)
		"DK": "Europe/Copenhagen",    // Denmark - CET (UTC+1/2)
		"NO": "Europe/Oslo",          // Norway - CET (UTC+1/2)
		"SE": "Europe/Stockholm",     // Sweden - CET (UTC+1/2)
		"FI": "Europe/Helsinki",      // Finland - EET (UTC+2/3)
		"PL": "Europe/Warsaw",        // Poland - CET (UTC+1/2)
		"CZ": "Europe/Prague",        // Czech Republic - CET (UTC+1/2)
		"SK": "Europe/Bratislava",    // Slovakia - CET (UTC+1/2)
		"HU": "Europe/Budapest",      // Hungary - CET (UTC+1/2)
		"RO": "Europe/Bucharest",     // Romania - EET (UTC+2/3)
		"BG": "Europe/Sofia",         // Bulgaria - EET (UTC+2/3)
		"RU": "Europe/Moscow",        // Russia - MSK (UTC+3, capital/most populous)
		"TR": "Europe/Istanbul",      // Turkey - TRT (UTC+3)
		"GR": "Europe/Athens",        // Greece - EET (UTC+2/3)
		"UA": "Europe/Kiev",          // Ukraine - EET (UTC+2/3)
		"HR": "Europe/Zagreb",        // Croatia - CET (UTC+1/2)
		"RS": "Europe/Belgrade",      // Serbia - CET (UTC+1/2)
		"SI": "Europe/Ljubljana",     // Slovenia - CET (UTC+1/2)

		// Africa
		"ZA": "Africa/Johannesburg",  // South Africa - SAST (UTC+2)
		"EG": "Africa/Cairo",         // Egypt - EET (UTC+2/3)
		"NG": "Africa/Lagos",         // Nigeria - WAT (UTC+1)
		"KE": "Africa/Nairobi",       // Kenya - EAT (UTC+3)

		// Oceania
		"AU": "Australia/Sydney",     // Australia - AEST (most populous)
		"NZ": "Pacific/Auckland",     // New Zealand - NZST (UTC+12/13)
	}

	timezone, ok := countryToTimezone[regionCode]
	if !ok {
		log.Printf("⚠️ Unknown country code %s for phone number %s, defaulting to Asia/Kolkata", regionCode, phoneNumber)
		return "Asia/Kolkata", nil // Default to IST
	}

	log.Printf("🌍 Detected timezone %s from phone number %s (country: %s)", timezone, phoneNumber, regionCode)
	return timezone, nil
}

// ConvertLocalToUTC converts a local datetime to UTC in RFC3339 format
// Accepts formats like:
// - "2025-11-09 14:30" (date + time)
// - "2025-11-09T14:30:00" (ISO format without timezone)
// Returns UTC time in RFC3339 format for database storage
func ConvertLocalToUTC(localDateTime string, phoneNumber string) (string, error) {
	// Detect timezone from phone number
	timezoneName, err := GetTimezoneFromPhoneNumber(phoneNumber)
	if err != nil {
		return "", fmt.Errorf("failed to detect timezone: %v", err)
	}

	// Load the detected timezone
	location, err := time.LoadLocation(timezoneName)
	if err != nil {
		return "", fmt.Errorf("failed to load timezone %s: %v", timezoneName, err)
	}

	// Try parsing different formats
	var parsedTime time.Time

	// Try format: "2025-11-09 14:30"
	parsedTime, err = time.ParseInLocation("2006-01-02 15:04", localDateTime, location)
	if err != nil {
		// Try format: "2025-11-09T14:30:00"
		parsedTime, err = time.ParseInLocation("2006-01-02T15:04:05", localDateTime, location)
		if err != nil {
			// Try format: "2025-11-09T14:30"
			parsedTime, err = time.ParseInLocation("2006-01-02T15:04", localDateTime, location)
			if err != nil {
				return "", fmt.Errorf("invalid datetime format: %s (use YYYY-MM-DD HH:MM or YYYY-MM-DDTHH:MM:SS)", localDateTime)
			}
		}
	}

	// Convert to UTC and format as RFC3339
	utcTime := parsedTime.UTC()
	log.Printf("⏰ Converted %s (%s) to UTC %s", localDateTime, timezoneName, utcTime.Format(time.RFC3339))
	return utcTime.Format(time.RFC3339), nil
}

// ConvertISTToUTC is deprecated - use ConvertLocalToUTC instead
// Kept for backwards compatibility
func ConvertISTToUTC(istDateTime string) (string, error) {
	return ConvertLocalToUTC(istDateTime, "+91") // Assume India number for backwards compatibility
}

// AddReminder creates a new reminder in Supabase
// reminderTime should be in local format like "2025-11-09 14:30" and will be converted to UTC based on phone number timezone
// recurrencePattern can be: empty/"once" (one-time), "daily", "weekly", "monthly", "yearly"
func AddReminder(reminderText, reminderTime, phoneNumber, recurrencePattern string) (*ZiggyReminder, error) {
	supabaseURL := os.Getenv("SUPABASE_URL")
	supabaseKey := os.Getenv("SUPABASE_ANON_KEY")

	if supabaseURL == "" || supabaseKey == "" {
		return nil, fmt.Errorf("Supabase credentials not configured")
	}

	// Convert local time to UTC for database storage (timezone auto-detected from phone number)
	utcTime, err := ConvertLocalToUTC(reminderTime, phoneNumber)
	if err != nil {
		return nil, fmt.Errorf("invalid reminder time: %v", err)
	}

	// Normalize recurrence pattern
	if recurrencePattern == "" {
		recurrencePattern = "once"
	}

	reminder := ZiggyReminder{
		PhoneNumber:       phoneNumber,
		ReminderText:      reminderText,
		ReminderTime:      utcTime, // Store as UTC
		RecurrencePattern: recurrencePattern,
		Status:            "pending",
	}

	jsonData, err := json.Marshal(reminder)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/rest/v1/ziggy_reminders", supabaseURL)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}

	req.Header.Set("apikey", supabaseKey)
	req.Header.Set("Authorization", "Bearer "+supabaseKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Prefer", "return=representation")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Supabase error: %s - %s", resp.Status, string(body))
	}

	var reminders []ZiggyReminder
	if err := json.Unmarshal(body, &reminders); err != nil {
		return nil, err
	}

	if len(reminders) == 0 {
		return nil, fmt.Errorf("no reminder returned from Supabase")
	}

	log.Printf("✅ Reminder created in Supabase: %s at %s (ID: %s)", reminders[0].ReminderText, reminders[0].ReminderTime, reminders[0].ID)
	return &reminders[0], nil
}

// GetDueReminders retrieves all pending reminders that are due
func GetDueReminders() ([]ZiggyReminder, error) {
	supabaseURL := os.Getenv("SUPABASE_URL")
	supabaseKey := os.Getenv("SUPABASE_ANON_KEY")

	if supabaseURL == "" || supabaseKey == "" {
		return nil, fmt.Errorf("Supabase credentials not configured")
	}

	// Query for pending reminders where reminder_time <= now
	url := fmt.Sprintf("%s/rest/v1/ziggy_reminders?status=eq.pending&reminder_time=lte.%s&order=reminder_time.asc",
		supabaseURL, time.Now().UTC().Format(time.RFC3339))

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

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Supabase error: %s - %s", resp.Status, string(body))
	}

	var reminders []ZiggyReminder
	if err := json.Unmarshal(body, &reminders); err != nil {
		return nil, err
	}

	log.Printf("📋 Retrieved %d due reminders from Supabase", len(reminders))
	return reminders, nil
}

// UpdateReminderStatus updates the status of a reminder
func UpdateReminderStatus(reminderID, status string, callID string) error {
	supabaseURL := os.Getenv("SUPABASE_URL")
	supabaseKey := os.Getenv("SUPABASE_ANON_KEY")

	if supabaseURL == "" || supabaseKey == "" {
		return fmt.Errorf("Supabase credentials not configured")
	}

	update := map[string]interface{}{
		"status": status,
	}

	if callID != "" {
		update["call_id"] = callID
	}

	jsonData, err := json.Marshal(update)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/rest/v1/ziggy_reminders?id=eq.%s", supabaseURL, reminderID)
	req, err := http.NewRequest("PATCH", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}

	req.Header.Set("apikey", supabaseKey)
	req.Header.Set("Authorization", "Bearer "+supabaseKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("Supabase error: %s - %s", resp.Status, string(body))
	}

	log.Printf("✅ Reminder %s updated to status: %s", reminderID, status)
	return nil
}

// ListReminders retrieves reminders for a phone number
func ListReminders(phoneNumber string, status string) ([]ZiggyReminder, error) {
	supabaseURL := os.Getenv("SUPABASE_URL")
	supabaseKey := os.Getenv("SUPABASE_ANON_KEY")

	if supabaseURL == "" || supabaseKey == "" {
		return nil, fmt.Errorf("Supabase credentials not configured")
	}

	url := fmt.Sprintf("%s/rest/v1/ziggy_reminders?phone_number=eq.%s&order=reminder_time.asc",
		supabaseURL, phoneNumber)

	if status != "" {
		url += fmt.Sprintf("&status=eq.%s", status)
	}

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

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Supabase error: %s - %s", resp.Status, string(body))
	}

	var reminders []ZiggyReminder
	if err := json.Unmarshal(body, &reminders); err != nil {
		return nil, err
	}

	log.Printf("📋 Retrieved %d reminders from Supabase for %s", len(reminders), phoneNumber)
	return reminders, nil
}

// CancelReminder cancels a reminder by updating its status to 'cancelled'
// The database trigger will automatically unschedule the cron job
func CancelReminder(reminderID string) error {
	return UpdateReminderStatus(reminderID, "cancelled", "")
}

// WhatsAppCallPermission represents a call permission record
type WhatsAppCallPermission struct {
	ID                       string `json:"id,omitempty"`
	PhoneNumber              string `json:"phone_number"`
	FirstInboundCallAt       string `json:"first_inbound_call_at,omitempty"`
	LastInboundCallAt        string `json:"last_inbound_call_at,omitempty"`
	PermissionGranted        bool   `json:"permission_granted"`
	TotalInboundCalls        int    `json:"total_inbound_calls,omitempty"`
	PermissionRequestedAt    string `json:"permission_requested_at,omitempty"`
	PermissionApprovedAt     string `json:"permission_approved_at,omitempty"`
	PermissionExpiresAt      string `json:"permission_expires_at,omitempty"` // 72 hours after approval
	PermissionRequestCount   int    `json:"permission_request_count,omitempty"`
	LastPermissionRequestAt  string `json:"last_permission_request_at,omitempty"`
	PermissionSource         string `json:"permission_source,omitempty"` // "inbound_call", "express_request", "manual"
	CreatedAt                string `json:"created_at,omitempty"`
	UpdatedAt                string `json:"updated_at,omitempty"`
}

// GrantCallPermission records that a user has granted call permission by calling us first
// This is called automatically when we receive an inbound call
// If the user already exists, it updates the last_inbound_call_at and increments the counter
func GrantCallPermission(phoneNumber string) error {
	supabaseURL := os.Getenv("SUPABASE_URL")
	supabaseKey := os.Getenv("SUPABASE_ANON_KEY")

	if supabaseURL == "" || supabaseKey == "" {
		return fmt.Errorf("Supabase credentials not configured")
	}

	// First, check if permission already exists
	existing, err := CheckCallPermission(phoneNumber)
	if err == nil && existing != nil {
		// Update existing record: increment counter and update last call time
		update := map[string]interface{}{
			"last_inbound_call_at": time.Now().UTC().Format(time.RFC3339),
			"total_inbound_calls":  existing.TotalInboundCalls + 1,
			"permission_granted":   true, // Re-grant if it was revoked
			"updated_at":           time.Now().UTC().Format(time.RFC3339),
		}

		jsonData, err := json.Marshal(update)
		if err != nil {
			return err
		}

		url := fmt.Sprintf("%s/rest/v1/whatsapp_call_permissions?phone_number=eq.%s", supabaseURL, phoneNumber)
		req, err := http.NewRequest("PATCH", url, bytes.NewBuffer(jsonData))
		if err != nil {
			return err
		}

		req.Header.Set("apikey", supabaseKey)
		req.Header.Set("Authorization", "Bearer "+supabaseKey)
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
			return fmt.Errorf("Supabase error updating permission: %s - %s", resp.Status, string(body))
		}

		log.Printf("✅ Updated call permission for %s (total calls: %d)", phoneNumber, existing.TotalInboundCalls+1)
		return nil
	}

	// Create new permission record
	permission := WhatsAppCallPermission{
		PhoneNumber:       phoneNumber,
		PermissionGranted: true,
		TotalInboundCalls: 1,
	}

	jsonData, err := json.Marshal(permission)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/rest/v1/whatsapp_call_permissions", supabaseURL)
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

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Supabase error creating permission: %s - %s", resp.Status, string(body))
	}

	log.Printf("✅ Granted call permission for %s (first inbound call)", phoneNumber)
	return nil
}

// CheckCallPermission checks if a phone number has permission to receive calls
// Returns the permission record if it exists and is granted, nil otherwise
// Also validates 72-hour expiry window for express permissions
func CheckCallPermission(phoneNumber string) (*WhatsAppCallPermission, error) {
	supabaseURL := os.Getenv("SUPABASE_URL")
	supabaseKey := os.Getenv("SUPABASE_ANON_KEY")

	if supabaseURL == "" || supabaseKey == "" {
		return nil, fmt.Errorf("Supabase credentials not configured")
	}

	url := fmt.Sprintf("%s/rest/v1/whatsapp_call_permissions?phone_number=eq.%s&permission_granted=eq.true",
		supabaseURL, phoneNumber)

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

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Supabase error: %s - %s", resp.Status, string(body))
	}

	var permissions []WhatsAppCallPermission
	if err := json.Unmarshal(body, &permissions); err != nil {
		return nil, err
	}

	if len(permissions) == 0 {
		return nil, nil // No permission found
	}

	permission := &permissions[0]

	// Check if permission has expired (72-hour window for express permissions)
	if permission.PermissionExpiresAt != "" {
		expiresAt, err := time.Parse(time.RFC3339, permission.PermissionExpiresAt)
		if err == nil && time.Now().UTC().After(expiresAt) {
			log.Printf("⚠️ Call permission for %s has expired (expired at %s)", phoneNumber, permission.PermissionExpiresAt)
			// Auto-revoke expired permission
			RevokeCallPermission(phoneNumber)
			return nil, nil // Permission expired
		}
	}

	// Permission is valid and not expired
	return permission, nil
}

// RevokeCallPermission revokes call permission for a phone number
// This can be called if a user opts out or requests to stop receiving calls
func RevokeCallPermission(phoneNumber string) error {
	supabaseURL := os.Getenv("SUPABASE_URL")
	supabaseKey := os.Getenv("SUPABASE_ANON_KEY")

	if supabaseURL == "" || supabaseKey == "" {
		return fmt.Errorf("Supabase credentials not configured")
	}

	update := map[string]interface{}{
		"permission_granted": false,
		"updated_at":         time.Now().UTC().Format(time.RFC3339),
	}

	jsonData, err := json.Marshal(update)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/rest/v1/whatsapp_call_permissions?phone_number=eq.%s", supabaseURL, phoneNumber)
	req, err := http.NewRequest("PATCH", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}

	req.Header.Set("apikey", supabaseKey)
	req.Header.Set("Authorization", "Bearer "+supabaseKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("Supabase error: %s - %s", resp.Status, string(body))
	}

	log.Printf("🚫 Revoked call permission for %s", phoneNumber)
	return nil
}

// RequestCallPermission sends an interactive message asking user for call permission
// Returns true if request was sent, false if rate limited
// Rate limits: 1 request per 24 hours, 2 requests per 7 days
func RequestCallPermission(phoneNumber string) (bool, error) {
	supabaseURL := os.Getenv("SUPABASE_URL")
	supabaseKey := os.Getenv("SUPABASE_ANON_KEY")

	if supabaseURL == "" || supabaseKey == "" {
		return false, fmt.Errorf("Supabase credentials not configured")
	}

	// Check existing permission record
	existing, _ := CheckCallPermission(phoneNumber)

	now := time.Now().UTC()

	// Check rate limits
	if existing != nil && existing.LastPermissionRequestAt != "" {
		lastRequest, err := time.Parse(time.RFC3339, existing.LastPermissionRequestAt)
		if err == nil {
			// Check 24-hour limit
			if now.Sub(lastRequest) < 24*time.Hour {
				log.Printf("🚫 Rate limited: Cannot request permission for %s (last request was %v ago)",
					phoneNumber, now.Sub(lastRequest))
				return false, fmt.Errorf("rate limited: must wait 24 hours between requests")
			}

			// Check 7-day limit (2 requests max)
			if existing.PermissionRequestCount >= 2 && now.Sub(lastRequest) < 7*24*time.Hour {
				log.Printf("🚫 Rate limited: Cannot request permission for %s (2 requests in past 7 days)", phoneNumber)
				return false, fmt.Errorf("rate limited: maximum 2 requests per 7 days")
			}
		}
	}

	// Create or update permission record with pending status
	if existing == nil {
		// Create new record
		permission := map[string]interface{}{
			"phone_number":               phoneNumber,
			"permission_granted":         false, // Pending approval
			"permission_requested_at":    now.Format(time.RFC3339),
			"last_permission_request_at": now.Format(time.RFC3339),
			"permission_request_count":   1,
			"permission_source":          "express_request",
		}

		jsonData, err := json.Marshal(permission)
		if err != nil {
			return false, err
		}

		url := fmt.Sprintf("%s/rest/v1/whatsapp_call_permissions", supabaseURL)
		req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
		if err != nil {
			return false, err
		}

		req.Header.Set("apikey", supabaseKey)
		req.Header.Set("Authorization", "Bearer "+supabaseKey)
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return false, err
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
			return false, fmt.Errorf("Supabase error: %s - %s", resp.Status, string(body))
		}
	} else {
		// Update existing record
		requestCount := existing.PermissionRequestCount + 1

		// Reset count if 7 days have passed
		if existing.LastPermissionRequestAt != "" {
			lastRequest, err := time.Parse(time.RFC3339, existing.LastPermissionRequestAt)
			if err == nil && now.Sub(lastRequest) >= 7*24*time.Hour {
				requestCount = 1
			}
		}

		update := map[string]interface{}{
			"permission_requested_at":    now.Format(time.RFC3339),
			"last_permission_request_at": now.Format(time.RFC3339),
			"permission_request_count":   requestCount,
			"updated_at":                 now.Format(time.RFC3339),
		}

		jsonData, err := json.Marshal(update)
		if err != nil {
			return false, err
		}

		url := fmt.Sprintf("%s/rest/v1/whatsapp_call_permissions?phone_number=eq.%s", supabaseURL, phoneNumber)
		req, err := http.NewRequest("PATCH", url, bytes.NewBuffer(jsonData))
		if err != nil {
			return false, err
		}

		req.Header.Set("apikey", supabaseKey)
		req.Header.Set("Authorization", "Bearer "+supabaseKey)
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return false, err
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
			return false, fmt.Errorf("Supabase error: %s - %s", resp.Status, string(body))
		}
	}

	log.Printf("✅ Recorded permission request for %s", phoneNumber)
	return true, nil
}

// ApproveCallPermission approves a call permission request
// Sets 72-hour expiry window from approval time
func ApproveCallPermission(phoneNumber, source string) error {
	supabaseURL := os.Getenv("SUPABASE_URL")
	supabaseKey := os.Getenv("SUPABASE_ANON_KEY")

	if supabaseURL == "" || supabaseKey == "" {
		return fmt.Errorf("Supabase credentials not configured")
	}

	now := time.Now().UTC()
	expiresAt := now.Add(72 * time.Hour) // 72-hour call window

	update := map[string]interface{}{
		"permission_granted":    true,
		"permission_approved_at": now.Format(time.RFC3339),
		"permission_expires_at":  expiresAt.Format(time.RFC3339),
		"permission_source":      source,
		"updated_at":             now.Format(time.RFC3339),
	}

	jsonData, err := json.Marshal(update)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/rest/v1/whatsapp_call_permissions?phone_number=eq.%s", supabaseURL, phoneNumber)
	req, err := http.NewRequest("PATCH", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}

	req.Header.Set("apikey", supabaseKey)
	req.Header.Set("Authorization", "Bearer "+supabaseKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("Supabase error: %s - %s", resp.Status, string(body))
	}

	log.Printf("✅ Approved call permission for %s (expires in 72 hours)", phoneNumber)
	return nil
}

// SendCallPermissionRequest sends an interactive message to request call permission
// Combines database tracking with actual WhatsApp message sending
func SendCallPermissionRequest(phoneNumber string) error {
	// First, check rate limits and record the request
	allowed, err := RequestCallPermission(phoneNumber)
	if err != nil {
		return err
	}
	if !allowed {
		return fmt.Errorf("rate limited")
	}

	// Get WhatsApp credentials
	token := os.Getenv("WHATSAPP_TOKEN")
	phoneID := os.Getenv("PHONE_NUMBER_ID")

	if token == "" || phoneID == "" {
		return fmt.Errorf("WhatsApp credentials not configured")
	}

	// Create WhatsApp SDK client
	config := &Config{
		Token:   token,
		PhoneID: phoneID,
	}
	client := NewWhatsAppClient(config)

	// Create permission request buttons
	buttons := []Button{
		NewButton("approve_call_permission", "✅ Yes, you can call me"),
		NewButton("deny_call_permission", "❌ No, thanks"),
	}

	// Send the interactive message
	body := "📞 Would you like to receive voice calls from us? This will allow us to contact you by phone when needed."

	_, err = client.SendButtons(phoneNumber, body, buttons, nil)
	if err != nil {
		log.Printf("❌ Failed to send permission request to %s: %v", phoneNumber, err)
		return fmt.Errorf("failed to send WhatsApp message: %v", err)
	}

	log.Printf("📤 Sent call permission request to %s", phoneNumber)
	return nil
}

// ZiggyNote represents a note created by the user
type ZiggyNote struct {
	ID          string `json:"id,omitempty"`
	PhoneNumber string `json:"phone_number"`
	NoteContent string `json:"note_content"`
}

// AddNote creates a new note in Supabase
func AddNote(noteContent, phoneNumber string) (*ZiggyNote, error) {
	supabaseURL := os.Getenv("SUPABASE_URL")
	supabaseKey := os.Getenv("SUPABASE_ANON_KEY")

	if supabaseURL == "" || supabaseKey == "" {
		return nil, fmt.Errorf("Supabase credentials not configured")
	}

	note := ZiggyNote{
		PhoneNumber: phoneNumber,
		NoteContent: noteContent,
	}

	jsonData, err := json.Marshal(note)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/rest/v1/ziggy_notes", supabaseURL)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}

	req.Header.Set("apikey", supabaseKey)
	req.Header.Set("Authorization", "Bearer "+supabaseKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Prefer", "return=representation")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Supabase error: %s - %s", resp.Status, string(body))
	}

	var notes []ZiggyNote
	if err := json.Unmarshal(body, &notes); err != nil {
		return nil, err
	}

	if len(notes) == 0 {
		return nil, fmt.Errorf("no note returned from Supabase")
	}

	log.Printf("✅ Note created in Supabase: %s (ID: %s)", notes[0].NoteContent, notes[0].ID)
	return &notes[0], nil
}

// ListNotes retrieves all notes for a phone number, ordered by most recent first
// If limit is provided and > 0, only returns that many notes
func ListNotes(phoneNumber string, limit int) ([]ZiggyNote, error) {
	supabaseURL := os.Getenv("SUPABASE_URL")
	supabaseKey := os.Getenv("SUPABASE_ANON_KEY")

	if supabaseURL == "" || supabaseKey == "" {
		return nil, fmt.Errorf("Supabase credentials not configured")
	}

	url := fmt.Sprintf("%s/rest/v1/ziggy_notes?phone_number=eq.%s&order=created_at.desc",
		supabaseURL, phoneNumber)

	if limit > 0 {
		url += fmt.Sprintf("&limit=%d", limit)
	}

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

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Supabase error: %s - %s", resp.Status, string(body))
	}

	var notes []ZiggyNote
	if err := json.Unmarshal(body, &notes); err != nil {
		return nil, err
	}

	log.Printf("📋 Retrieved %d notes from Supabase for %s", len(notes), phoneNumber)
	return notes, nil
}

// SearchNotes searches notes for a phone number using full-text search
func SearchNotes(phoneNumber, searchQuery string) ([]ZiggyNote, error) {
	supabaseURL := os.Getenv("SUPABASE_URL")
	supabaseKey := os.Getenv("SUPABASE_ANON_KEY")

	if supabaseURL == "" || supabaseKey == "" {
		return nil, fmt.Errorf("Supabase credentials not configured")
	}

	// Use PostgreSQL full-text search with to_tsvector and plainto_tsquery
	// Format: note_content.fts(english).searchQuery
	url := fmt.Sprintf("%s/rest/v1/ziggy_notes?phone_number=eq.%s&note_content=fts.%s",
		supabaseURL, phoneNumber, searchQuery)

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

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Supabase error: %s - %s", resp.Status, string(body))
	}

	var notes []ZiggyNote
	if err := json.Unmarshal(body, &notes); err != nil {
		return nil, err
	}

	log.Printf("🔍 Found %d notes matching '%s' for %s", len(notes), searchQuery, phoneNumber)
	return notes, nil
}

// UpdateNote updates the content of an existing note
func UpdateNote(noteID, newContent string) error {
	supabaseURL := os.Getenv("SUPABASE_URL")
	supabaseKey := os.Getenv("SUPABASE_ANON_KEY")

	if supabaseURL == "" || supabaseKey == "" {
		return fmt.Errorf("Supabase credentials not configured")
	}

	update := map[string]string{
		"note_content": newContent,
	}

	jsonData, err := json.Marshal(update)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/rest/v1/ziggy_notes?id=eq.%s", supabaseURL, noteID)
	req, err := http.NewRequest("PATCH", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}

	req.Header.Set("apikey", supabaseKey)
	req.Header.Set("Authorization", "Bearer "+supabaseKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("Supabase error: %s - %s", resp.Status, string(body))
	}

	log.Printf("✅ Note %s updated", noteID)
	return nil
}

// DeleteNote deletes a note by ID
func DeleteNote(noteID string) error {
	supabaseURL := os.Getenv("SUPABASE_URL")
	supabaseKey := os.Getenv("SUPABASE_ANON_KEY")

	if supabaseURL == "" || supabaseKey == "" {
		return fmt.Errorf("Supabase credentials not configured")
	}

	url := fmt.Sprintf("%s/rest/v1/ziggy_notes?id=eq.%s", supabaseURL, noteID)
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return err
	}

	req.Header.Set("apikey", supabaseKey)
	req.Header.Set("Authorization", "Bearer "+supabaseKey)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("Supabase error: %s - %s", resp.Status, string(body))
	}

	log.Printf("🗑️ Note %s deleted", noteID)
	return nil
}
