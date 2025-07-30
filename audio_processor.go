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

	// For now, we'll forward the raw Opus payload
	// In a production system, you'd decode Opus to PCM16 here
	// The payload is already Opus-encoded audio from WhatsApp
	opusData := packet.Payload

	// OpenAI expects PCM16 audio, but for initial testing,
	// let's try sending the Opus data and see if OpenAI handles it
	// If not, we'll need to add an Opus decoder

	// Convert to base64 (OpenAI expects base64-encoded audio)
	audioBase64 := base64.StdEncoding.EncodeToString(opusData)

	// Send to OpenAI via data channel
	if err := p.openAIClient.SendAudioToOpenAI([]byte(audioBase64)); err != nil {
		if p.packetCount < 5 {
			log.Printf("❌ Failed to send audio to OpenAI: %v", err)
		}
		return err
	}

	p.packetCount++
	if p.packetCount == 1 {
		log.Printf("✅ First audio packet sent to OpenAI data channel!")
	} else if p.packetCount%100 == 0 {
		log.Printf("📊 Sent %d audio packets to OpenAI via data channel", p.packetCount)
	}

	// Commit buffer periodically (every 100ms)
	if time.Since(p.lastCommitTime) > 100*time.Millisecond {
		if err := p.openAIClient.CommitAudioBuffer(); err != nil {
			log.Printf("❌ Failed to commit audio buffer: %v", err)
		} else {
			if p.packetCount < 1000 { // Log first few commits
				log.Printf("✅ Committed audio buffer to OpenAI")
			}
		}
		p.lastCommitTime = time.Now()
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