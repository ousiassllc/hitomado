package siwc

import (
	"net/url"
	"strings"
)

// authorizeURL は認可 URL。target は再ログインする登録（新規登録なら零値）。
func (m *Manager) authorizeURL(authEndpoint string, p pendingAuth, hostID string, target account, idTokenHint string) string {
	clientID := target.ClientID
	q := url.Values{}
	if clientID == "" {
		clientID = dynamicClientID
		// agent_name_hint は初回の動的登録だけに付ける（発行済みの client_id での再認可には付けない）。
		q.Set("agent_name_hint", AgentName)
	}
	q.Set("client_id", clientID)
	q.Set("ext_agent_host_id", hostID)
	q.Set("response_type", "code")
	q.Set("redirect_uri", p.redirectURI)
	q.Set("scope", requestScopes)
	q.Set("resource", resource)
	q.Set("state", p.state)
	q.Set("nonce", p.nonce)
	q.Set("code_challenge_method", "S256")
	q.Set("code_challenge", pkceChallenge(p.verifier))
	if target.ClientID != "" {
		if idTokenHint != "" {
			q.Set("id_token_hint", idTokenHint)
		}
		if target.Email != "" {
			q.Set("login_hint", target.Email)
		}
		if target.ConsentNeeded {
			// 前回 ChatGPT プランの利用が許可されなかった。利用者が改めてログインを選んだので、同意を求め直す。
			q.Set("prompt", "consent")
		}
	}
	return authEndpointWithQuery(authEndpoint, q)
}

func authEndpointWithQuery(endpoint string, q url.Values) string {
	sep := "?"
	if strings.Contains(endpoint, "?") {
		sep = "&"
	}
	return endpoint + sep + q.Encode()
}
