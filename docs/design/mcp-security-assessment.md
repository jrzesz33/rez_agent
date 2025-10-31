# MCP Server Security & Risk Assessment

## 1. Executive Summary

This document identifies security requirements, potential vulnerabilities, compliance needs, and risk mitigation strategies for the MCP (Model Context Protocol) server implementation in the rez_agent system.

**Risk Level: MEDIUM**
- External API integrations (Golf, Weather)
- User data handling (reservations, personal info)
- Financial transactions (tee time bookings)
- Public HTTP endpoint exposure

## 2. Security Requirements

### 2.1 Authentication & Authorization

**Requirements:**
- ✅ **API Key Authentication** (Phase 1)
  - Stdio client authenticates with Lambda via API key
  - API keys stored in AWS Secrets Manager
  - Key rotation every 90 days

- ✅ **JWT Authentication** (Phase 2 - Future)
  - OAuth2 flow for end-user authentication
  - JWT tokens with 1-hour expiration
  - Refresh token mechanism

- ✅ **Per-Tool Authorization**
  - Read-only tools: No special authorization
  - Write operations (booking): Requires verified user
  - Admin tools: Not exposed via MCP

**Implementation:**
```go
type AuthContext struct {
    ClientID    string
    UserID      string
    Permissions []string
    IssuedAt    time.Time
    ExpiresAt   time.Time
}
```

### 2.2 Data Privacy & Protection

**PII Data Handled:**
- User names
- Email addresses
- Phone numbers (for reservations)
- Golf course booking details

**Protection Measures:**
- ✅ Encryption in transit: TLS 1.2+ for all HTTP traffic
- ✅ Encryption at rest: DynamoDB encryption enabled
- ✅ PII redaction in logs
- ✅ Data retention: 24-hour TTL on request tracking
- ✅ No credit card data stored (delegated to golf course API)

**Logging Security:**
```go
// Redact sensitive fields
type SafeLogEntry struct {
    Tool      string `json:"tool"`
    UserID    string `json:"user_id"` // Hash or redact
    Timestamp string `json:"timestamp"`
    Status    string `json:"status"`
    // NEVER log: API keys, passwords, full user details
}
```

### 2.3 Input Validation & Sanitization

**Validation Layers:**
1. **JSON-RPC Layer**: Validate JSON-RPC 2.0 message structure
2. **Schema Validation**: JSON schema validation for tool inputs
3. **Business Logic**: Semantic validation (e.g., date in future)
4. **Output Sanitization**: HTML/script injection prevention

**Attack Vectors to Prevent:**
- ✅ SQL Injection: Use parameterized queries (DynamoDB API handles this)
- ✅ XSS: Sanitize all text outputs before returning to client
- ✅ Command Injection: No shell commands executed with user input
- ✅ Path Traversal: Not applicable (no file system access)
- ✅ SSRF: Validate external API URLs (whitelist only)

### 2.4 Rate Limiting & DoS Protection

**Rate Limits:**
```
Per Client:
  - Normal tools: 100 requests/hour
  - Booking tools: 10 requests/hour
  - Read-only tools: 200 requests/hour

Global:
  - Lambda concurrency: Max 10 concurrent executions
  - API Gateway throttle: 1000 requests/second
```

**Implementation:**
- DynamoDB-based rate limiting table
- Exponential backoff for retries
- Circuit breaker for external APIs

### 2.5 API Security (External Integrations)

**Golf Course API:**
- ✅ OAuth2 with client credentials grant
- ✅ Credentials stored in Secrets Manager
- ✅ Auto-rotation where supported
- ✅ Least privilege scopes

**Weather API:**
- ✅ API key authentication
- ✅ HTTPS only
- ✅ Rate limiting respected (1000 calls/day)

**ntfy.sh:**
- ⚠️ Public endpoint (no auth required)
- ⚠️ Topic name is publicly accessible
- ✅ Mitigation: Use unique, hard-to-guess topic names
- ✅ Future: Self-hosted ntfy with authentication

## 3. Compliance & Regulatory Requirements

### 3.1 OWASP Top 10 (2021) Compliance

| Risk | Mitigation |
|------|------------|
| A01: Broken Access Control | API key auth, per-tool authorization, rate limiting |
| A02: Cryptographic Failures | TLS 1.2+, DynamoDB encryption, Secrets Manager |
| A03: Injection | Parameterized queries, input validation, output sanitization |
| A04: Insecure Design | Security by design, threat modeling, this assessment |
| A05: Security Misconfiguration | IAM least privilege, S3 public access blocked |
| A06: Vulnerable Components | Dependency scanning (go mod, Dependabot) |
| A07: Identification & Authentication | API keys + JWT, no default credentials |
| A08: Software & Data Integrity | Code signing, immutable artifacts, checksums |
| A09: Security Logging Monitoring | CloudWatch, X-Ray, structured logging |
| A10: SSRF | URL whitelist for external APIs only |

### 3.2 Data Protection (GDPR/CCPA Considerations)

**Applicability:** If users are in EU/California:
- ✅ Data minimization: Only collect necessary data
- ✅ Right to access: API to retrieve user data
- ✅ Right to deletion: TTL-based auto-deletion (24 hours)
- ✅ Data portability: Export user data on request
- ⚠️ Consent: Obtain explicit consent for data collection

**Implementation:**
- Privacy policy disclosure
- User consent tracking
- Data deletion API endpoint

### 3.3 PCI DSS (Payment Card Industry)

**Applicability:** NOT APPLICABLE
- No credit card data stored
- Payments handled by golf course API (delegated responsibility)
- If future payments: Use PCI-compliant payment gateway (Stripe, etc.)

## 4. Threat Modeling

### 4.1 STRIDE Analysis

#### S - Spoofing
**Threat:** Attacker impersonates legitimate client
- **Likelihood:** Medium
- **Impact:** High (unauthorized bookings, data access)
- **Mitigation:** API key authentication, IP whitelist (optional), JWT tokens

#### T - Tampering
**Threat:** Attacker modifies requests/responses in transit
- **Likelihood:** Low (HTTPS)
- **Impact:** High
- **Mitigation:** TLS 1.2+, HTTPS everywhere, certificate pinning (client-side)

#### R - Repudiation
**Threat:** User denies making a booking
- **Likelihood:** Medium
- **Impact:** Medium
- **Mitigation:** Audit logging, request tracking, immutable DynamoDB records

#### I - Information Disclosure
**Threat:** Sensitive data leaked via logs, errors, or responses
- **Likelihood:** Medium
- **Impact:** High (PII exposure)
- **Mitigation:** PII redaction, error message sanitization, secure logging

#### D - Denial of Service
**Threat:** Attacker overwhelms system with requests
- **Likelihood:** Medium
- **Impact:** Medium (service unavailable)
- **Mitigation:** Rate limiting, Lambda concurrency limits, API Gateway throttling

#### E - Elevation of Privilege
**Threat:** Normal user gains admin access or accesses others' data
- **Likelihood:** Low
- **Impact:** High
- **Mitigation:** IAM least privilege, per-request authorization checks, user context isolation

### 4.2 Attack Surface Analysis

**External-Facing Components:**
- ✅ API Gateway: `/mcp`, `/mcp/status/{id}`, `/mcp/stream`
- ✅ Stdio Client: Runs locally (low risk)
- ⚠️ ntfy.sh: Public notification endpoint

**Attack Vectors:**
1. **Malicious stdio client** → Validated by API key
2. **Compromised API key** → Key rotation, rate limiting
3. **Golf API credential leak** → Secrets Manager, auto-rotation
4. **DDoS on API Gateway** → Throttling, WAF (future)
5. **Injection attacks** → Input validation, parameterized queries

## 5. Risk Assessment Matrix

| Risk | Likelihood | Impact | Severity | Mitigation |
|------|-----------|--------|----------|------------|
| Unauthorized booking | Medium | High | **HIGH** | API key auth, user context validation |
| PII data leak | Medium | High | **HIGH** | PII redaction, encryption, audit logs |
| API key compromise | Low | High | **MEDIUM** | Key rotation, rate limiting, monitoring |
| DDoS attack | Medium | Medium | **MEDIUM** | Rate limiting, Lambda concurrency cap |
| Golf API credential leak | Low | Medium | **LOW** | Secrets Manager, least privilege IAM |
| Weather API abuse | Low | Low | **LOW** | Rate limiting, caching |
| ntfy.sh topic hijacking | Medium | Low | **LOW** | Unique topic names, future: self-host |
| XSS in responses | Low | Medium | **LOW** | Output sanitization |
| SQL injection | Very Low | High | **LOW** | DynamoDB API (no SQL) |
| SSRF to internal AWS | Very Low | High | **LOW** | URL whitelist, VPC isolation |

## 6. Security Controls Implementation

### 6.1 Preventative Controls

```go
// API Key Validation
func validateAPIKey(ctx context.Context, key string) (*AuthContext, error) {
    // 1. Check key format (prevent injection)
    if !isValidKeyFormat(key) {
        return nil, ErrInvalidKeyFormat
    }

    // 2. Retrieve from Secrets Manager
    secret, err := getSecret(ctx, "mcp-api-keys")
    if err != nil {
        return nil, err
    }

    // 3. Constant-time comparison (prevent timing attacks)
    if !subtle.ConstantTimeCompare([]byte(key), []byte(secret.Key)) {
        return nil, ErrUnauthorized
    }

    // 4. Check expiration
    if time.Now().After(secret.ExpiresAt) {
        return nil, ErrKeyExpired
    }

    return &AuthContext{ClientID: secret.ClientID}, nil
}

// Input Validation
func validateToolInput(toolName string, input map[string]interface{}) error {
    schema := getToolSchema(toolName)
    validator := jsonschema.NewValidator(schema)

    if err := validator.Validate(input); err != nil {
        return fmt.Errorf("invalid input: %w", err)
    }

    // Additional business logic validation
    switch toolName {
    case "golf_book_tee_time":
        return validateBookingInput(input)
    }

    return nil
}

// Rate Limiting
func checkRateLimit(ctx context.Context, clientID, toolName string) error {
    bucket := fmt.Sprintf("%s:%s:%s", clientID, toolName, time.Now().Format("2006-01-02-15"))

    count, err := incrementCounter(ctx, bucket)
    if err != nil {
        return err
    }

    limit := getRateLimitForTool(toolName)
    if count > limit {
        return ErrRateLimitExceeded
    }

    return nil
}
```

### 6.2 Detective Controls

**CloudWatch Alarms:**
```
- High error rate (>5% in 5 minutes)
- Unusual API call volume (>1000 in 1 hour)
- Multiple failed auth attempts (>10 in 1 minute)
- External API failures (>10% in 5 minutes)
- Lambda errors (any invocation error)
```

**X-Ray Tracing:**
- Track all requests end-to-end
- Identify slow external API calls
- Detect anomalous patterns

**Audit Logging:**
```json
{
  "event": "tool_invocation",
  "timestamp": "2025-10-31T12:00:00Z",
  "client_id": "client-123",
  "user_id_hash": "sha256-hash",
  "tool": "golf_book_tee_time",
  "status": "success",
  "ip_address": "203.0.113.42"
}
```

### 6.3 Corrective Controls

**Incident Response Plan:**
1. **Detection** → CloudWatch alarm triggers
2. **Assessment** → Review logs, X-Ray traces
3. **Containment** → Revoke compromised API keys, block IPs
4. **Eradication** → Patch vulnerabilities, rotate secrets
5. **Recovery** → Restore service, verify functionality
6. **Lessons Learned** → Post-mortem, update security controls

**Automated Responses:**
- API key auto-revocation after 10 failed auth attempts
- IP ban after 100 requests in 1 minute
- Circuit breaker opens after 5 consecutive external API failures

## 7. Secrets Management

### 7.1 Secrets Inventory

| Secret | Purpose | Rotation | Access |
|--------|---------|----------|--------|
| mcp-api-keys | Client authentication | 90 days | MCP Lambda |
| golf-api-credentials | Golf course API OAuth | 180 days | MCP Lambda |
| weather-api-key | Weather API | 365 days | MCP Lambda |
| jwt-signing-key | JWT token signing | 180 days | MCP Lambda (future) |

### 7.2 Secret Rotation Strategy

```go
// Auto-rotation Lambda (triggered every 90 days)
func rotateAPIKeys(ctx context.Context) error {
    // 1. Generate new key
    newKey := generateSecureAPIKey()

    // 2. Store new key in Secrets Manager (versioned)
    if err := storeSecret(ctx, "mcp-api-keys", newKey); err != nil {
        return err
    }

    // 3. Notify clients of upcoming rotation (7 days notice)
    sendRotationNotification(ctx, newKey.ActivatesAt)

    // 4. After grace period, revoke old key
    time.Sleep(7 * 24 * time.Hour)
    if err := revokeOldKey(ctx); err != nil {
        return err
    }

    return nil
}
```

## 8. Network Security

### 8.1 VPC Configuration (Future Enhancement)

**Current:** Lambda in default VPC (internet access)
**Recommended:** Move to private VPC with NAT Gateway

```
┌─────────────────────────────────────┐
│         Public Subnet               │
│  ┌──────────────┐                   │
│  │ NAT Gateway  │                   │
│  └──────────────┘                   │
└─────────────────────────────────────┘
         │
┌─────────────────────────────────────┐
│        Private Subnet                │
│  ┌──────────────┐                   │
│  │ MCP Lambda   │                   │
│  └──────────────┘                   │
│         │                            │
│         ├─> DynamoDB (VPC Endpoint)  │
│         ├─> Secrets Mgr (VPC Endpt)  │
│         └─> Internet (via NAT)       │
└─────────────────────────────────────┘
```

### 8.2 TLS/SSL Configuration

**Requirements:**
- TLS 1.2 minimum
- Strong cipher suites only
- Certificate validation enforced
- No self-signed certificates in production

**API Gateway:**
- AWS-managed certificate
- Custom domain (optional): `mcp.rez-agent.com`
- HTTPS redirect enabled

## 9. Dependency Security

### 9.1 Go Module Scanning

**Tools:**
- `go list -m all` → List all dependencies
- `nancy` or `govulncheck` → Vulnerability scanning
- GitHub Dependabot → Automated updates

**Policy:**
- No dependencies with known high/critical CVEs
- Monthly dependency updates
- Pin major versions, allow minor/patch updates

### 9.2 Container Security (Docker)

**Base Image:**
- `public.ecr.aws/lambda/python:3.12` (Python agent)
- `scratch` or `alpine` (Go binaries)

**Best Practices:**
- ✅ Use official AWS Lambda base images
- ✅ Multi-stage builds (reduce attack surface)
- ✅ No secrets in images
- ✅ Scan images with `docker scan` or Trivy

## 10. Monitoring & Alerting

### 10.1 Security Monitoring

**CloudWatch Logs Insights Queries:**

```sql
-- Failed authentication attempts
fields @timestamp, client_id, ip_address
| filter event = "auth_failed"
| stats count() by client_id
| filter count > 10

-- Unusual tool invocations
fields @timestamp, tool, client_id
| filter tool = "golf_book_tee_time"
| stats count() by client_id
| filter count > 5

-- PII in logs (should be 0)
fields @message
| filter @message like /email|phone|ssn/
```

### 10.2 Alert Thresholds

| Alert | Threshold | Action |
|-------|-----------|--------|
| Failed auth rate | >10/min | Block client, notify admin |
| Error rate | >5% | Investigate, rollback if needed |
| API latency | P95 >2s | Scale up, check external APIs |
| Concurrent executions | >8 | Check for DDoS, increase limit |
| Secret rotation overdue | >100 days | Immediate rotation |

## 11. Incident Response Procedures

### 11.1 Security Incident Severity Levels

**P0 - Critical:**
- Active data breach
- Compromised AWS credentials
- Unauthorized access to production

**P1 - High:**
- Compromised API keys
- PII data exposure
- Service completely down

**P2 - Medium:**
- Failed security audit
- Dependency vulnerability (high/critical CVE)
- Partial service degradation

**P3 - Low:**
- Security misconfiguration
- Non-critical vulnerability
- Failed compliance check

### 11.2 Response Procedures

**For P0/P1 Incidents:**
1. **Immediate Actions (0-15 minutes):**
   - Revoke compromised credentials
   - Enable WAF rules to block attack
   - Notify security team

2. **Containment (15-60 minutes):**
   - Isolate affected resources
   - Collect forensic data
   - Assess impact scope

3. **Eradication (1-4 hours):**
   - Patch vulnerabilities
   - Rotate all secrets
   - Update security controls

4. **Recovery (4-24 hours):**
   - Restore service
   - Verify no backdoors
   - Monitor for re-compromise

5. **Post-Incident (1-7 days):**
   - Root cause analysis
   - Update runbooks
   - Security training

## 12. Compliance Checklist

- [ ] API key authentication implemented
- [ ] Rate limiting configured
- [ ] PII redaction in logs verified
- [ ] Secrets stored in AWS Secrets Manager
- [ ] TLS 1.2+ enforced
- [ ] DynamoDB encryption enabled
- [ ] IAM least privilege policies
- [ ] CloudWatch alarms configured
- [ ] X-Ray tracing enabled
- [ ] Input validation for all tools
- [ ] Output sanitization implemented
- [ ] Dependency scanning automated
- [ ] Incident response plan documented
- [ ] Security testing completed
- [ ] Penetration testing (if required)
- [ ] Privacy policy published
- [ ] User consent mechanism (if GDPR applies)

## 13. Security Testing Plan

### 13.1 Static Application Security Testing (SAST)

**Tools:**
- `gosec` - Go security checker
- `go vet` - Built-in linter
- SonarQube - Code quality & security

### 13.2 Dynamic Application Security Testing (DAST)

**Tools:**
- OWASP ZAP - Web application scanner
- Burp Suite - Manual penetration testing

**Test Cases:**
- SQL injection attempts
- XSS payloads in tool inputs
- Authentication bypass attempts
- Rate limiting enforcement
- API key leakage in responses

### 13.3 Dependency Scanning

```bash
# Go vulnerability check
go install golang.org/x/vuln/cmd/govulncheck@latest
govulncheck ./...

# Nancy (Sonatype OSS Index)
go list -json -m all | nancy sleuth
```

## 14. Recommendations & Action Items

### 14.1 Immediate (Pre-Launch)

- [ ] Implement API key authentication
- [ ] Configure rate limiting
- [ ] Enable PII redaction in logs
- [ ] Set up CloudWatch alarms
- [ ] Store all secrets in Secrets Manager
- [ ] Run dependency vulnerability scan

### 14.2 Short-Term (Month 1)

- [ ] Implement JWT authentication
- [ ] Add IP whitelisting (optional)
- [ ] Set up automated secret rotation
- [ ] Conduct security code review
- [ ] Run OWASP ZAP scan
- [ ] Document incident response procedures

### 14.3 Long-Term (Quarter 1)

- [ ] Move Lambda to private VPC
- [ ] Self-host ntfy.sh with authentication
- [ ] Implement WAF rules (AWS WAF)
- [ ] Conduct penetration testing
- [ ] SOC 2 compliance (if required)
- [ ] Security training for team

## 15. Conclusion

The MCP server implementation presents a **MEDIUM** security risk level, primarily due to external API integrations and handling of user personal information. With the implemented security controls (API key auth, rate limiting, encryption, input validation, PII redaction), the residual risk is **LOW to MEDIUM** and acceptable for initial deployment.

**Critical Success Factors:**
1. Proper API key management and rotation
2. Comprehensive input validation
3. PII redaction in all logs and errors
4. Rate limiting to prevent abuse
5. Continuous monitoring and alerting

**Sign-off:**
- Security assessment completed: 2025-10-31
- Next review date: 2025-11-30 (30 days post-launch)
