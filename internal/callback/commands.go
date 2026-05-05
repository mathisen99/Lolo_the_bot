package callback

import (
	"fmt"
	"strings"
	"time"

	"gopkg.in/irc.v4"
)

// executeWHOIS executes a WHOIS command and collects the response
func (s *Server) executeWHOIS(network string, args []string) (string, error) {
	if len(args) < 1 {
		return "", fmt.Errorf("whois requires a nickname")
	}
	executor, _, execErr := s.executorFor(network)
	if execErr != nil {
		return "", execErr
	}
	nick := args[0]

	collector := s.startCollectingForNetwork(network, "whois_"+nick, 5*time.Second)
	defer s.stopCollectingForNetwork(network, "whois_"+nick)

	err := executor.Write(&irc.Message{
		Command: "WHOIS",
		Params:  []string{nick},
	})
	if err != nil {
		return "", fmt.Errorf("failed to send WHOIS: %w", err)
	}

	return collector.Wait(), nil
}

// executeWHOWAS executes a WHOWAS command
func (s *Server) executeWHOWAS(network string, args []string) (string, error) {
	if len(args) < 1 {
		return "", fmt.Errorf("whowas requires a nickname")
	}
	executor, _, execErr := s.executorFor(network)
	if execErr != nil {
		return "", execErr
	}
	nick := args[0]

	collector := s.startCollectingForNetwork(network, "whowas_"+nick, 5*time.Second)
	defer s.stopCollectingForNetwork(network, "whowas_"+nick)

	err := executor.Write(&irc.Message{
		Command: "WHOWAS",
		Params:  []string{nick},
	})
	if err != nil {
		return "", fmt.Errorf("failed to send WHOWAS: %w", err)
	}

	return collector.Wait(), nil
}

// executeCTCP sends a CTCP request (VERSION, TIME, etc.)
func (s *Server) executeCTCP(network string, args []string, ctcpType string) (string, error) {
	if len(args) < 1 {
		return "", fmt.Errorf("%s requires a nickname", strings.ToLower(ctcpType))
	}
	executor, _, execErr := s.executorFor(network)
	if execErr != nil {
		return "", execErr
	}
	nick := args[0]

	collector := s.startCollectingForNetwork(network, "ctcp_"+nick, 10*time.Second)
	defer s.stopCollectingForNetwork(network, "ctcp_"+nick)

	// CTCP is sent as PRIVMSG with \x01TYPE\x01
	ctcpMsg := fmt.Sprintf("\x01%s\x01", ctcpType)
	err := executor.SendMessage(nick, ctcpMsg)
	if err != nil {
		return "", fmt.Errorf("failed to send CTCP %s: %w", ctcpType, err)
	}

	return collector.Wait(), nil
}

// executeNickServInfo queries NickServ INFO
func (s *Server) executeNickServInfo(network string, args []string) (string, error) {
	if len(args) < 1 {
		return "", fmt.Errorf("nickserv info requires a nickname")
	}
	executor, _, execErr := s.executorFor(network)
	if execErr != nil {
		return "", execErr
	}
	nick := args[0]

	collector := s.startCollectingForNetwork(network, "nickserv", 10*time.Second)
	defer s.stopCollectingForNetwork(network, "nickserv")

	err := executor.SendMessage("NickServ", fmt.Sprintf("INFO %s", nick))
	if err != nil {
		return "", fmt.Errorf("failed to send NickServ INFO: %w", err)
	}

	return collector.Wait(), nil
}

// executeNickServCommand executes a NickServ command (GHOST, RELEASE, REGAIN)
func (s *Server) executeNickServCommand(network string, cmd string, args []string) (string, error) {
	if len(args) < 1 {
		return "", fmt.Errorf("nickserv %s requires a nickname", strings.ToLower(cmd))
	}
	executor, _, execErr := s.executorFor(network)
	if execErr != nil {
		return "", execErr
	}

	collector := s.startCollectingForNetwork(network, "nickserv", 10*time.Second)
	defer s.stopCollectingForNetwork(network, "nickserv")

	fullCmd := fmt.Sprintf("%s %s", cmd, strings.Join(args, " "))
	err := executor.SendMessage("NickServ", fullCmd)
	if err != nil {
		return "", fmt.Errorf("failed to send NickServ %s: %w", cmd, err)
	}

	return collector.Wait(), nil
}

// executeChanServInfo queries ChanServ INFO
func (s *Server) executeChanServInfo(network string, args []string) (string, error) {
	if len(args) < 1 {
		return "", fmt.Errorf("chanserv info requires a channel")
	}
	executor, _, execErr := s.executorFor(network)
	if execErr != nil {
		return "", execErr
	}
	channel := args[0]

	collector := s.startCollectingForNetwork(network, "chanserv", 10*time.Second)
	defer s.stopCollectingForNetwork(network, "chanserv")

	err := executor.SendMessage("ChanServ", fmt.Sprintf("INFO %s", channel))
	if err != nil {
		return "", fmt.Errorf("failed to send ChanServ INFO: %w", err)
	}

	return collector.Wait(), nil
}

// executeChanServCommand executes a ChanServ command
func (s *Server) executeChanServCommand(network string, cmd string, args []string) (string, error) {
	if len(args) < 1 {
		return "", fmt.Errorf("chanserv %s requires at least a channel", strings.ToLower(cmd))
	}
	executor, _, execErr := s.executorFor(network)
	if execErr != nil {
		return "", execErr
	}

	collector := s.startCollectingForNetwork(network, "chanserv", 10*time.Second)
	defer s.stopCollectingForNetwork(network, "chanserv")

	fullCmd := fmt.Sprintf("%s %s", cmd, strings.Join(args, " "))
	err := executor.SendMessage("ChanServ", fullCmd)
	if err != nil {
		return "", fmt.Errorf("failed to send ChanServ %s: %w", cmd, err)
	}

	return collector.Wait(), nil
}

// executeALIS searches channels via ALIS service
func (s *Server) executeALIS(network string, args []string) (string, error) {
	if len(args) < 1 {
		return "", fmt.Errorf("alis search requires a pattern")
	}
	executor, _, execErr := s.executorFor(network)
	if execErr != nil {
		return "", execErr
	}

	collector := s.startCollectingForNetwork(network, "alis", 15*time.Second)
	defer s.stopCollectingForNetwork(network, "alis")

	// ALIS LIST <pattern> [-min N] [-max N] [-topic] etc.
	fullCmd := fmt.Sprintf("LIST %s", strings.Join(args, " "))
	err := executor.SendMessage("ALIS", fullCmd)
	if err != nil {
		return "", fmt.Errorf("failed to send ALIS LIST: %w", err)
	}

	return collector.Wait(), nil
}

// executeKick kicks a user from a channel
func (s *Server) executeKick(network string, args []string) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("kick requires channel and nickname")
	}
	executor, _, execErr := s.executorFor(network)
	if execErr != nil {
		return "", execErr
	}
	channel := args[0]
	nick := args[1]
	reason := "Kicked"
	if len(args) > 2 {
		reason = strings.Join(args[2:], " ")
	}

	err := executor.Write(&irc.Message{
		Command: "KICK",
		Params:  []string{channel, nick, reason},
	})
	if err != nil {
		return "", fmt.Errorf("failed to kick: %w", err)
	}

	return fmt.Sprintf("Kicked %s from %s: %s", nick, channel, reason), nil
}

// executeMode sets a channel mode
func (s *Server) executeMode(network string, args []string, mode string) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("mode command requires channel and target")
	}
	executor, _, execErr := s.executorFor(network)
	if execErr != nil {
		return "", execErr
	}
	channel := args[0]
	target := args[1]

	err := executor.Write(&irc.Message{
		Command: "MODE",
		Params:  []string{channel, mode, target},
	})
	if err != nil {
		return "", fmt.Errorf("failed to set mode: %w", err)
	}

	return fmt.Sprintf("Set mode %s %s on %s", mode, target, channel), nil
}

// executeRawMode sets arbitrary channel modes
func (s *Server) executeRawMode(network string, args []string) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("mode requires channel and modes")
	}
	executor, _, execErr := s.executorFor(network)
	if execErr != nil {
		return "", execErr
	}
	channel := args[0]
	modes := args[1:]

	params := append([]string{channel}, modes...)
	err := executor.Write(&irc.Message{
		Command: "MODE",
		Params:  params,
	})
	if err != nil {
		return "", fmt.Errorf("failed to set mode: %w", err)
	}

	return fmt.Sprintf("Set mode %s on %s", strings.Join(modes, " "), channel), nil
}

// executeTopic sets a channel topic
func (s *Server) executeTopic(network string, args []string) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("topic requires channel and new topic")
	}
	executor, _, execErr := s.executorFor(network)
	if execErr != nil {
		return "", execErr
	}
	channel := args[0]
	topic := strings.Join(args[1:], " ")

	err := executor.Write(&irc.Message{
		Command: "TOPIC",
		Params:  []string{channel, topic},
	})
	if err != nil {
		return "", fmt.Errorf("failed to set topic: %w", err)
	}

	return fmt.Sprintf("Topic set on %s", channel), nil
}

// executeInvite invites a user to a channel
func (s *Server) executeInvite(network string, args []string) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("invite requires channel and nickname")
	}
	executor, _, execErr := s.executorFor(network)
	if execErr != nil {
		return "", execErr
	}
	channel := args[0]
	nick := args[1]

	err := executor.Write(&irc.Message{
		Command: "INVITE",
		Params:  []string{nick, channel},
	})
	if err != nil {
		return "", fmt.Errorf("failed to invite: %w", err)
	}

	return fmt.Sprintf("Invited %s to %s", nick, channel), nil
}
