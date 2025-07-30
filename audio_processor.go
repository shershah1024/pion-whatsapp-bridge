package main

import (
	"encoding/base64"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/pion/rtp"
)

// AudioProcessor handles conversion between RTP and OpenAI's expected format
type AudioProcessor struct {
	openAIClient      *OpenAIRealtimeClient
	rtpBuffer         []byte
	pcmBuffer         []byte
	audioBuffer       []byte  // Accumulated audio to send
	lastCommitTime    time.Time
	mu                sync.Mutex
	packetCount       int
	isProcessing      bool
	stopChan          chan bool
	lastSequence      uint16
	firstPacket       bool
}

// NewAudioProcessor creates a new audio processor
func NewAudioProcessor(client *OpenAIRealtimeClient) *AudioProcessor {
	return &AudioProcessor{
		openAIClient:   client,
		rtpBuffer:      make([]byte, 1500),
		pcmBuffer:      make([]byte, 0, 48000), // 1 second buffer at 48kHz
		audioBuffer:    make([]byte, 0, 96000), // Buffer for accumulating audio before sending
		lastCommitTime: time.Now(),
		stopChan:       make(chan bool),
		firstPacket:    true,
	}
}

// ProcessRTPPacket handles incoming RTP packets from WhatsApp
func (p *AudioProcessor) ProcessRTPPacket(rtpData []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Parse RTP packet
	packet := &rtp.Packet{}
	if err := packet.Unmarshal(rtpData); err != nil {
		return fmt.Errorf("failed to parse RTP packet: %v", err)
	}

	// Check for packet loss
	if !p.firstPacket {
		expectedSeq := p.lastSequence + 1
		if packet.SequenceNumber != expectedSeq {
			log.Printf("⚠️ Packet loss detected: expected seq %d, got %d", expectedSeq, packet.SequenceNumber)
		}
	}
	p.lastSequence = packet.SequenceNumber
	p.firstPacket = false

	// OpenAI expects PCM16 audio, not Opus
	// For now, let's send silence PCM16 data to test the connection
	// In production, we'd decode the Opus payload to PCM16
	
	// Generate 20ms of PCM16 silence (960 samples at 48kHz)
	pcm16Data := generateSilencePCM16(20)
	
	// Accumulate audio in buffer
	p.audioBuffer = append(p.audioBuffer, pcm16Data...)
	
	p.packetCount++
	if p.packetCount == 1 {
		log.Printf("✅ First audio packet received from WhatsApp!")
	}

	// Send accumulated audio every 100ms (5 packets of 20ms each)
	if len(p.audioBuffer) >= 9600 { // 100ms of audio at 48kHz, 16-bit mono = 9600 bytes
		// Convert accumulated audio to base64
		audioBase64 := base64.StdEncoding.EncodeToString(p.audioBuffer)
		
		// Send to OpenAI
		if err := p.openAIClient.SendAudioToOpenAI([]byte(audioBase64)); err != nil {
			if p.packetCount < 10 {
				log.Printf("❌ Failed to send audio to OpenAI: %v", err)
			}
			return err
		}
		
		// Commit the buffer
		if err := p.openAIClient.CommitAudioBuffer(); err != nil {
			log.Printf("❌ Failed to commit audio buffer: %v", err)
		} else {
			log.Printf("✅ Sent and committed %d bytes of audio to OpenAI", len(p.audioBuffer))
		}
		
		// Clear the buffer for next batch
		p.audioBuffer = p.audioBuffer[:0]
		p.lastCommitTime = time.Now()
	}
	
	if p.packetCount%100 == 0 {
		log.Printf("📊 Processed %d packets from WhatsApp", p.packetCount)
	}
	
	// After 3 seconds, trigger a manual response if we haven't heard anything
	if p.packetCount == 150 { // 150 packets = 3 seconds at 20ms intervals
		log.Printf("🎯 Triggering manual OpenAI response after 3 seconds")
		go func() {
			if err := p.openAIClient.TriggerResponse("I can hear you. How can I help you today?"); err != nil {
				log.Printf("❌ Failed to trigger manual response: %v", err)
			}
		}()
	}

	return nil
}

// Start begins processing
func (p *AudioProcessor) Start() {
	p.mu.Lock()
	if p.isProcessing {
		p.mu.Unlock()
		return
	}
	p.isProcessing = true
	p.mu.Unlock()

	log.Printf("🎙️ Audio processor started")
}

// Stop halts processing
func (p *AudioProcessor) Stop() {
	p.mu.Lock()
	if !p.isProcessing {
		p.mu.Unlock()
		return
	}
	p.isProcessing = false
	p.mu.Unlock()

	close(p.stopChan)
	log.Printf("🛑 Audio processor stopped")
}

// Simple PCM16 silence generator for testing
func generateSilencePCM16(durationMs int) []byte {
	// 48kHz, 16-bit, mono
	sampleRate := 48000
	samples := (sampleRate * durationMs) / 1000
	pcm := make([]byte, samples*2) // 2 bytes per sample
	
	// Fill with zeros (silence)
	return pcm
}

// Helper function to convert Opus to PCM16 (placeholder)
// In production, you'd use an Opus decoder library
func opusToPCM16(opusData []byte) ([]byte, error) {
	// TODO: Implement actual Opus decoding
	// For now, return silence
	return generateSilencePCM16(20), nil // 20ms of silence
}