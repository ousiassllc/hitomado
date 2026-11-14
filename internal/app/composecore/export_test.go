package composecore

// ForceSenderForTest はセッションの送信元だけを差し替える（SENDER_MISMATCH の防御検査を再現するため。テスト専用）。
func (s *Service) ForceSenderForTest(sessionID, accountID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[sessionID].accountID = accountID
}

// SendingForTest はセッションが送信処理中か（テスト専用。送信が io ロック待ちに入ったことの同期に使う）。
func (s *Service) SendingForTest(sessionID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	cs, ok := s.sessions[sessionID]
	return ok && cs.sending
}

// SessionCountForTest は開いている作成セッションの数（テスト専用。画面を介さない下書きづくりがセッションを残さないことの確認に使う）。
func (s *Service) SessionCountForTest() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.sessions)
}
