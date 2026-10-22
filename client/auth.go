package client

import clientauth "kc/client/auth"

type Config = clientauth.Config
type Identity = clientauth.Identity
type Authentication = clientauth.Authentication
type LoginRequest = clientauth.LoginRequest
type Session = clientauth.Session
type Authenticator = clientauth.Authenticator
type PassThroughAuthenticator = clientauth.PassThroughAuthenticator
type SessionStore = clientauth.SessionStore
type MemorySessionStore = clientauth.MemorySessionStore
type RequestOptions = clientauth.RequestOptions
type IdentityService = clientauth.IdentityService
type AuthDiscovery = clientauth.AuthDiscovery
type BrowserLoginConfig = clientauth.BrowserLoginConfig
type TokenRequest = clientauth.TokenRequest
type TokenResponse = clientauth.TokenResponse
