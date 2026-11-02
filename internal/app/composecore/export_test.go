package composecore

// ForceSenderForTest はセッションの送信元だけを差し替える（SENDER_MISMATCH の防御検査を再現するため。テスト専用）。
func (s *Service) ForceSenderForTest(sessionID, accountID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[sessionID].accountID = accountID
}
