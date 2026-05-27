package service

import "time"

func openAIStreamInitialBufferExpired(firstPendingAt, now time.Time, timeout time.Duration) bool {
	return timeout > 0 && !firstPendingAt.IsZero() && !now.Before(firstPendingAt.Add(timeout))
}

func (s *OpenAIGatewayService) streamInitialBufferTimeout() time.Duration {
	if s == nil || s.cfg == nil {
		return time.Second
	}
	ms := s.cfg.Gateway.StreamInitialBufferTimeoutMS
	if ms < 0 {
		return 0
	}
	if ms == 0 {
		return 0
	}
	return time.Duration(ms) * time.Millisecond
}
