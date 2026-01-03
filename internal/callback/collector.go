package callback

import (
	"strings"
	"time"
)

// startCollecting starts collecting responses for a request
func (s *Server) startCollecting(key string, timeout time.Duration) *ResponseCollector {
	collector := &ResponseCollector{
		responses: make([]string, 0),
		done:      make(chan struct{}),
		timeout:   timeout,
	}

	s.mu.Lock()
	s.pendingRequests[key] = collector
	s.mu.Unlock()

	// Auto-complete after timeout
	go func() {
		time.Sleep(timeout)
		collector.Complete()
	}()

	return collector
}

// stopCollecting removes a collector
func (s *Server) stopCollecting(key string) {
	s.mu.Lock()
	delete(s.pendingRequests, key)
	s.mu.Unlock()
}

// AddResponse adds a response line to a collector
func (c *ResponseCollector) AddResponse(line string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	select {
	case <-c.done:
		// Already completed
		return
	default:
		c.responses = append(c.responses, line)
	}
}

// Complete marks the collector as done
func (c *ResponseCollector) Complete() {
	c.mu.Lock()
	defer c.mu.Unlock()

	select {
	case <-c.done:
		// Already closed
	default:
		close(c.done)
	}
}

// Wait waits for collection to complete and returns the result
func (c *ResponseCollector) Wait() string {
	<-c.done

	c.mu.Lock()
	defer c.mu.Unlock()

	return strings.Join(c.responses, "\n")
}

// HandleServiceResponse routes incoming service responses to the appropriate collector
// This should be called from the IRC message handler when receiving NOTICEs
func (s *Server) HandleServiceResponse(source, message string) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sourceLower := strings.ToLower(source)

	// Route based on source
	var collector *ResponseCollector

	switch sourceLower {
	case "nickserv":
		collector = s.pendingRequests["nickserv"]
	case "chanserv":
		collector = s.pendingRequests["chanserv"]
	case "alis":
		collector = s.pendingRequests["alis"]
	default:
		// Check for CTCP responses or WHOIS numerics
		for key, c := range s.pendingRequests {
			if strings.HasPrefix(key, "ctcp_") || strings.HasPrefix(key, "whois_") || strings.HasPrefix(key, "whowas_") {
				collector = c
				break
			}
		}
	}

	if collector != nil {
		collector.AddResponse(message)
	}
}

// HandleNumericResponse handles numeric IRC responses (WHOIS, WHOWAS, etc.)
func (s *Server) HandleNumericResponse(numeric int, params []string) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// WHOIS numerics: 311-318, 330, 338, 378, 379
	// WHOWAS numerics: 314, 369
	isWhois := (numeric >= 311 && numeric <= 318) || numeric == 330 || numeric == 338 || numeric == 378 || numeric == 379
	isWhowas := numeric == 314 || numeric == 369

	if !isWhois && !isWhowas {
		return
	}

	// Find the appropriate collector
	for key, collector := range s.pendingRequests {
		if (isWhois && strings.HasPrefix(key, "whois_")) || (isWhowas && strings.HasPrefix(key, "whowas_")) {
			// Format the response nicely
			if len(params) > 1 {
				message := formatNumericResponse(numeric, params)
				collector.AddResponse(message)
			}

			// End of WHOIS/WHOWAS
			if numeric == 318 || numeric == 369 {
				collector.Complete()
			}
			break
		}
	}
}

// formatNumericResponse formats a numeric response into a readable string
func formatNumericResponse(numeric int, params []string) string {
	// Skip the first param (usually our nick) and join the rest
	if len(params) <= 1 {
		return ""
	}

	switch numeric {
	case 311: // RPL_WHOISUSER
		// nick user host * :realname
		if len(params) >= 6 {
			return params[1] + " (" + params[2] + "@" + params[3] + "): " + params[5]
		}
	case 312: // RPL_WHOISSERVER
		// nick server :serverinfo
		if len(params) >= 4 {
			return "Server: " + params[2] + " (" + params[3] + ")"
		}
	case 313: // RPL_WHOISOPERATOR
		if len(params) >= 3 {
			return params[1] + " " + params[2]
		}
	case 317: // RPL_WHOISIDLE
		// nick idle signon :seconds idle, signon time
		if len(params) >= 4 {
			return "Idle: " + params[2] + " seconds, Signon: " + params[3]
		}
	case 319: // RPL_WHOISCHANNELS
		// nick :channels
		if len(params) >= 3 {
			return "Channels: " + params[2]
		}
	case 330: // RPL_WHOISACCOUNT (Libera)
		// nick account :is logged in as
		if len(params) >= 4 {
			return "Account: " + params[2]
		}
	case 338: // RPL_WHOISACTUALLY
		if len(params) >= 3 {
			return "Actually: " + params[2]
		}
	case 378: // RPL_WHOISHOST
		if len(params) >= 3 {
			return params[2]
		}
	case 314: // RPL_WHOWASUSER
		if len(params) >= 6 {
			return params[1] + " was (" + params[2] + "@" + params[3] + "): " + params[5]
		}
	}

	// Default: join remaining params
	return strings.Join(params[1:], " ")
}

// HandleCTCPResponse handles CTCP responses
func (s *Server) HandleCTCPResponse(source, ctcpType, response string) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := "ctcp_" + source
	if collector, ok := s.pendingRequests[key]; ok {
		collector.AddResponse(ctcpType + ": " + response)
		collector.Complete()
	}
}
