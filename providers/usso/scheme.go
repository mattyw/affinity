/*
   Affinity - Private groups as a service
   Copyright (C) 2014  Canonical, Ltd.

   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU Library General Public License as published by
   the Free Software Foundation, version 3.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU Library General Public License for more details.

   You should have received a copy of the GNU Library General Public License
   along with this program.  If not, see <http://www.gnu.org/licenses/>.
*/

package usso

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"

	"launchpad.net/usso"

	. "github.com/juju/affinity"
	"github.com/juju/affinity/providers/common"
)

type UssoScheme struct {
	PasswordProvider PasswordProvider
	Token            string
	OpenID           *common.OpenID
}

func NewOpenIdWeb(scheme string, token string) *UssoScheme {
	return &UssoScheme{
		PasswordProvider: &PasswordUnavailable{},
		Token:            token,
		OpenID:           common.NewSimpleOpenID(scheme, token),
	}
}

func NewOauthCli(token string) *UssoScheme {
	return &UssoScheme{
		PasswordProvider: &PasswordPrompter{},
		Token:            token,
	}
}

func (s *UssoScheme) Callback(w http.ResponseWriter, r *http.Request) (User, url.Values, error) {
	if s.OpenID == nil {
		return User{}, nil, fmt.Errorf("OpenID not supported")
	}

	var user User
	values := make(map[string][]string)
	err := fmt.Errorf("Failed to authenticate")
	s.OpenID.Callback(w, r, func(id map[string]string) {
		user = User{Identity{Scheme: s.Name(), Id: id["email"]}}
		for k, v := range id {
			values[k] = []string{v}
		}
		err = nil
	})
	return user, values, err
}

func (s *UssoScheme) Authenticate(w http.ResponseWriter, r *http.Request) bool {
	if s.OpenID == nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		log.Println(fmt.Errorf("OpenID not supported"))
		return false
	}

	rv, err := s.OpenID.Authenticate(usso.ProductionUbuntuSSOServer.LoginURL(), w, r)
	if err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		log.Println(err)
		return false
	}
	return rv
}

func (s *UssoScheme) Auth(id string) (values url.Values, err error) {
	pass, err := s.PasswordProvider.Password()
	if err != nil {
		return nil, err
	}

	ssoData, err := usso.ProductionUbuntuSSOServer.GetToken(id, pass, s.Token)
	if err != nil {
		return nil, err
	}

	return url.Values{
		"ConsumerKey":     []string{ssoData.ConsumerKey},
		"ConsumerSecret":  []string{ssoData.ConsumerSecret},
		"TokenKey":        []string{ssoData.TokenKey},
		"TokenName":       []string{ssoData.TokenName},
		"TokenSecret":     []string{ssoData.TokenSecret},
		"Affinity-Scheme": []string{s.Name()},
	}, nil
}

func (s *UssoScheme) Validate(values url.Values) (id string, err error) {
	consumerKey := values.Get("ConsumerKey")
	if consumerKey == "" {
		err = fmt.Errorf("No ConsumerKey provided in authorization")
		return "", err
	}
	consumerSecret := values.Get("ConsumerSecret")
	if consumerSecret == "" {
		err = fmt.Errorf("No ConsumerSecret provided in authorization")
		return "", err
	}
	tokenKey := values.Get("TokenKey")
	if tokenKey == "" {
		err = fmt.Errorf("No TokenKey provided in authorization")
		return "", err
	}
	tokenSecret := values.Get("TokenSecret")
	if tokenSecret == "" {
		err = fmt.Errorf("No TokenSecret provided in authorization")
		return "", err
	}
	tokenName := values.Get("TokenName")
	if tokenName == "" {
		err = fmt.Errorf("No TokenName provided in authorization")
		return "", err
	}
	// construct sso data collection for validation
	ssoData := usso.SSOData{
		ConsumerKey:    consumerKey,
		ConsumerSecret: consumerSecret,
		TokenKey:       tokenKey,
		TokenSecret:    tokenSecret,
		TokenName:      tokenName,
	}
	resultRaw, err := usso.ProductionUbuntuSSOServer.GetAccounts(&ssoData)
	if err != nil {
		log.Printf("Failed to validate USSO token data: %v", err)
		return "", err
	}
	result := map[string]interface{}{}
	err = json.Unmarshal([]byte(resultRaw), &result)
	if err != nil {
		log.Printf("Failed to decode USSO data: %v", err)
		return "", err
	}

	// check if the USS response has the necessary fields
	_, hasEmail := result["email"]
	_, hasDisplayName := result["displayname"]
	_, hasTokens := result["tokens"]
	if !hasEmail || !hasDisplayName || !hasTokens {
		err = fmt.Errorf("SSO validation failed, missing required fields")
		return "", err
	}
	email, ok := result["email"].(string)
	if !ok || email == "" {
		err = fmt.Errorf("Invalid SSO data received for %v", result["email"])
		return "", err
	}
	return email, nil
}

func (s *UssoScheme) Authorizer() SchemeAuthorizer { return s }

func (s *UssoScheme) Authenticator() SchemeAuthenticator { return s }

func (s *UssoScheme) Validator() SchemeValidator { return s }

func (s *UssoScheme) Name() string { return "usso" }
