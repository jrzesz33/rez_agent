# Web Action Processor - Security Implementation Checklist

**Quick reference guide for developers implementing the Web Action Processor**

---

## 🔴 CRITICAL: Must Complete Before Any Deployment

### SSRF Prevention (V-001)

```go
// ✅ DO THIS
func validateURL(rawURL string) error {
    allowedHosts := map[string]bool{
        "api.weather.gov":     true,
        "birdsfoot.cps.golf":  true,
    }

    parsed, err := url.Parse(rawURL)
    if err != nil {
        return fmt.Errorf("invalid URL: %w", err)
    }

    // Check 1: HTTPS only
    if parsed.Scheme != "https" {
        return fmt.Errorf("only HTTPS allowed")
    }

    // Check 2: Allowlist
    if !allowedHosts[parsed.Hostname()] {
        return fmt.Errorf("host not allowed: %s", parsed.Hostname())
    }

    // Check 3: Resolve and validate IP
    ips, err := net.LookupIP(parsed.Hostname())
    if err != nil {
        return fmt.Errorf("DNS lookup failed: %w", err)
    }

    for _, ip := range ips {
        // Block private IPs
        if isPrivateIP(ip) {
            return fmt.Errorf("private IP blocked")
        }

        // Block AWS metadata
        if ip.String() == "169.254.169.254" {
            return fmt.Errorf("AWS metadata blocked")
        }
    }

    return nil
}

// ❌ NEVER DO THIS
func processURL(rawURL string) {
    resp, _ := http.Get(rawURL)  // NO VALIDATION!
}
```

**Checklist:**
- [ ] URL validation function implemented
- [ ] Private IP ranges blocked (10.x, 192.168.x, 127.x, 169.254.x)
- [ ] AWS metadata endpoint blocked (169.254.169.254)
- [ ] HTTPS-only enforcement
- [ ] DNS rebinding protection (validate IP after resolving)
- [ ] Unit tests for all attack vectors
- [ ] Integration test with malicious URLs

---

### OAuth Token Logging Prevention (V-002)

```go
// ✅ DO THIS - Redact tokens
func logHTTPRequest(ctx context.Context, req *http.Request) {
    headers := make(map[string]string)
    for k, v := range req.Header {
        if k == "Authorization" {
            headers[k] = "[REDACTED]"
        } else {
            headers[k] = strings.Join(v, ",")
        }
    }

    logger.InfoContext(ctx, "HTTP request",
        slog.String("method", req.Method),
        slog.String("url", redactURL(req.URL.String())),
        slog.Any("headers", headers),
    )
}

// ✅ DO THIS - Log OAuth events without tokens
func logOAuthAttempt(ctx context.Context, success bool, err error) {
    logger.InfoContext(ctx, "oauth_authentication",
        slog.Bool("success", success),
        slog.String("error_type", getErrorType(err)),  // NOT full error
    )
}

// ❌ NEVER DO THIS
logger.InfoContext(ctx, "OAuth response",
    slog.String("token", accessToken))  // TOKEN LEAKED!

logger.ErrorContext(ctx, "Auth failed",
    slog.String("error", err.Error()))  // May contain credentials!
```

**Checklist:**
- [ ] Logging wrapper that redacts `Authorization` headers
- [ ] No logging of `access_token`, `refresh_token`, `password`, `secret`
- [ ] Error messages sanitized (no credential substrings)
- [ ] Static analysis added to CI/CD (`grep -r "slog.*token"`)
- [ ] CloudWatch Logs Insights query for token detection
- [ ] Manual log review after first deployment

---

## 🟠 HIGH PRIORITY: Complete Within 4 Weeks

### DynamoDB Encryption (V-003)

```go
// ✅ DO THIS - Pulumi infrastructure
kmsKey, err := kms.NewKey(ctx, "web-action-results-key", &kms.KeyArgs{
    Description:          pulumi.String("Encryption for web action results"),
    EnableKeyRotation:    pulumi.Bool(true),
    DeletionWindowInDays: pulumi.Int(30),
})

resultsTable, err := dynamodb.NewTable(ctx, "web-action-results", &dynamodb.TableArgs{
    Name: pulumi.String("rez-agent-web-action-results-prod"),
    ServerSideEncryption: &dynamodb.TableServerSideEncryptionArgs{
        Enabled:   pulumi.Bool(true),
        KmsKeyArn: kmsKey.Arn,
    },
    // ... rest of config
})

// ❌ NEVER DO THIS
resultsTable, err := dynamodb.NewTable(ctx, "web-action-results", &dynamodb.TableArgs{
    // No encryption configured!
})
```

**Checklist:**
- [ ] KMS customer-managed key created
- [ ] DynamoDB table encryption enabled
- [ ] CloudWatch Logs encryption enabled
- [ ] Encryption validated with AWS CLI
- [ ] KMS key rotation enabled (automatic yearly)

---

### IAM Least Privilege (V-005)

```go
// ✅ DO THIS - Specific resource ARNs
{
  "Effect": "Allow",
  "Action": "secretsmanager:GetSecretValue",
  "Resource": "arn:aws:secretsmanager:us-east-1:123456789012:secret:rez-agent/golf/credentials-AbCdEf",
  "Condition": {
    "StringEquals": {"aws:RequestedRegion": "us-east-1"}
  }
}

// ❌ NEVER DO THIS - Wildcard access
{
  "Effect": "Allow",
  "Action": "secretsmanager:GetSecretValue",
  "Resource": "arn:aws:secretsmanager:*:*:secret:rez-agent/*"
}
```

**Checklist:**
- [ ] Secrets Manager permission scoped to specific secret ARN
- [ ] DynamoDB permissions use specific table ARNs
- [ ] CloudWatch Logs permission scoped to function's log group
- [ ] No wildcard regions (`*` → `us-east-1`)
- [ ] No wildcard accounts (`*` → specific account ID)
- [ ] Unused permissions removed (`dynamodb:Query` if not needed)
- [ ] IAM Policy Simulator tested

---

### Secrets Rotation (V-006)

```go
// ✅ DO THIS - Automatic rotation
golfSecret, err := secretsmanager.NewSecret(ctx, "golf-credentials", &secretsmanager.SecretArgs{
    Name: pulumi.String("rez-agent/golf/credentials"),
    KmsKeyId: kmsKey.ID(),
    RotationRules: &secretsmanager.SecretRotationRulesArgs{
        AutomaticallyAfterDays: pulumi.Int(90),
    },
})

rotationLambda := lambda.NewFunction(ctx, "secrets-rotation", &lambda.FunctionArgs{
    // Lambda that rotates Golf API password
})

secretsmanager.NewSecretRotation(ctx, "golf-rotation", &secretsmanager.SecretRotationArgs{
    SecretId: golfSecret.ID(),
    RotationLambdaArn: rotationLambda.Arn,
})

// ❌ NEVER DO THIS - No rotation
golfSecret, err := secretsmanager.NewSecret(ctx, "golf-credentials", &secretsmanager.SecretArgs{
    // No rotation configured
})
```

**Checklist:**
- [ ] Rotation Lambda implemented (createSecret, setSecret, testSecret, finishSecret)
- [ ] Rotation scheduled (90 days)
- [ ] Rotation tested in dev environment
- [ ] CloudWatch alarm for rotation failures
- [ ] Zero-downtime rotation validated

---

### Security Event Logging (V-007)

```go
// ✅ DO THIS - Log all security events
type SecurityEvent struct {
    EventType    string    `json:"event_type"`
    Success      bool      `json:"success"`
    Principal    string    `json:"principal"`
    Resource     string    `json:"resource"`
    ErrorCode    string    `json:"error_code"`
    CorrelationID string   `json:"correlation_id"`
}

func logSecurityEvent(ctx context.Context, event SecurityEvent) {
    logger.InfoContext(ctx, "SECURITY_EVENT",
        slog.String("event_type", event.EventType),
        slog.Bool("success", event.Success),
        slog.String("resource", event.Resource),
        slog.String("error_code", event.ErrorCode),
        slog.String("correlation_id", event.CorrelationID),
    )
}

// Required events:
// - oauth_authentication_attempt
// - secret_accessed
// - url_validation_failed
// - http_request_executed
// - action_handler_invoked

// ❌ NEVER DO THIS - Incomplete logging
func authenticate() {
    token, err := oauthClient.GetToken()
    if err != nil {
        return err  // No security event logged!
    }
}
```

**Checklist:**
- [ ] OAuth authentication attempts logged (success/failure)
- [ ] Secrets Manager access logged
- [ ] URL validation failures logged
- [ ] HTTP requests logged (redacted)
- [ ] Action handler invocations logged
- [ ] CloudWatch dashboard created for security events
- [ ] CloudWatch alarms configured

---

## 🟡 MEDIUM PRIORITY: Complete Within 6 Weeks

### Input Validation (V-010)

```go
// ✅ DO THIS - Validate all inputs
func (p *WebActionPayload) Validate() error {
    if p.Version == "" {
        return fmt.Errorf("version required")
    }

    if err := validateURL(p.URL); err != nil {
        return fmt.Errorf("invalid URL: %w", err)
    }

    if p.Action == "" {
        return fmt.Errorf("action required")
    }

    // Validate arguments
    if days, ok := p.Arguments["days"].(float64); ok {
        if days < 1 || days > 14 {
            return fmt.Errorf("days must be 1-14, got %v", days)
        }
    }

    if maxResults, ok := p.Arguments["max_results"].(float64); ok {
        if maxResults < 1 || maxResults > 100 {
            return fmt.Errorf("max_results must be 1-100, got %v", maxResults)
        }
    }

    if golferID, ok := p.Arguments["golfer_id"].(string); ok {
        if !regexp.MustCompile(`^\d+$`).MatchString(golferID) {
            return fmt.Errorf("golfer_id must be numeric")
        }
    }

    return nil
}

// ❌ NEVER DO THIS - No validation
func processAction(payload *WebActionPayload) {
    days := payload.Arguments["days"].(int)  // Panic if not int!
    // Use days without validation
}
```

**Checklist:**
- [ ] URL validation
- [ ] Action type validation (allowlist)
- [ ] Arguments validated (type, range, format)
- [ ] Auth config validated
- [ ] Payload version checked
- [ ] Edge cases tested (negative values, excessive values, missing fields)

---

### HTTP Client Security (V-008, V-009)

```go
// ✅ DO THIS - Secure HTTP client
func NewSecureHTTPClient(timeout time.Duration) *http.Client {
    return &http.Client{
        Timeout: timeout,
        Transport: &http.Transport{
            TLSClientConfig: &tls.Config{
                MinVersion: tls.VersionTLS13,  // TLS 1.3 only
                MaxVersion: tls.VersionTLS13,
            },
            MaxIdleConns:        10,
            MaxIdleConnsPerHost: 2,
            IdleConnTimeout:     90 * time.Second,
        },
        CheckRedirect: func(req *http.Request, via []*http.Request) error {
            if len(via) >= 2 {
                return fmt.Errorf("too many redirects")
            }
            // Validate redirect URL
            if err := validateURL(req.URL.String()); err != nil {
                return err
            }
            return nil
        },
    }
}

// ❌ NEVER DO THIS - Insecure defaults
client := &http.Client{
    Timeout: 30 * time.Second,  // No TLS config, unlimited redirects
}
```

**Checklist:**
- [ ] TLS 1.3 enforced (or TLS 1.2 minimum for compatibility)
- [ ] Certificate validation enabled (no `InsecureSkipVerify`)
- [ ] Redirect limit set (max 2)
- [ ] Redirect URLs validated
- [ ] Connection pooling limits configured
- [ ] Response size limits (10MB max)
- [ ] Timeout configured (30s)

---

## 🟢 LOW PRIORITY: Future Improvements

### Certificate Pinning
```go
tlsConfig := &tls.Config{
    VerifyPeerCertificate: func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
        // Validate certificate fingerprint
    },
}
```

### Token Cache Encryption
```go
// Encrypt tokens in memory
encryptedToken := encryptInMemory([]byte(token))
```

---

## Pre-Deployment Testing Checklist

### Static Analysis (SAST)
```bash
# Run gosec
gosec -fmt json -out gosec-report.json ./...

# Check for token logging
grep -r "slog.*token\|logger.*Bearer" cmd/ internal/

# Dependency vulnerabilities
go list -json -m all | nancy sleuth
```

**Checklist:**
- [ ] gosec: 0 high/critical findings
- [ ] No token logging detected
- [ ] No known vulnerabilities in dependencies

---

### Dynamic Testing (DAST)

**SSRF Tests:**
```bash
# Test 1: AWS metadata
curl -X POST /api/messages -d '{"url":"http://169.254.169.254/latest/meta-data/"}'
# Expected: 400 Bad Request

# Test 2: Private IP
curl -X POST /api/messages -d '{"url":"http://192.168.1.1/"}'
# Expected: 400 Bad Request

# Test 3: Non-allowlisted host
curl -X POST /api/messages -d '{"url":"https://evil.com/"}'
# Expected: 400 Bad Request
```

**OAuth Tests:**
```bash
# Test 4: Check CloudWatch Logs for tokens
aws logs tail /aws/lambda/rez-agent-webaction-prod --follow | grep -i "Bearer"
# Expected: No matches

# Test 5: Invalid credentials
# Modify secret temporarily, verify no credential leakage in logs
```

**IAM Tests:**
```bash
# Test 6: IAM Policy Simulator
aws iam simulate-principal-policy \
  --policy-source-arn arn:aws:iam::ACCOUNT:role/rez-agent-webaction-role \
  --action-names secretsmanager:PutSecretValue \
  --resource-arns arn:aws:secretsmanager:*:*:secret:rez-agent/golf/credentials
# Expected: "denied"
```

**Checklist:**
- [ ] All SSRF tests passed
- [ ] No tokens in logs (verified manually)
- [ ] IAM permissions validated
- [ ] Input validation edge cases tested
- [ ] Error handling tested (no secrets leaked)

---

## Security Code Review Checklist

**For Reviewers:**

### HTTP Requests
- [ ] URL validated before EVERY HTTP request
- [ ] HTTPS-only enforced
- [ ] Redirect validation implemented
- [ ] Response size limited
- [ ] Timeout configured

### Logging
- [ ] No `slog.*token` or `slog.*password` or `slog.*secret`
- [ ] Authorization headers redacted
- [ ] Error messages sanitized
- [ ] Security events logged

### Secrets
- [ ] Secrets retrieved from Secrets Manager (not env vars)
- [ ] Secrets never logged
- [ ] Secrets not stored in DynamoDB
- [ ] OAuth tokens cached with expiration

### Input Validation
- [ ] All payload fields validated
- [ ] Type checking before type assertions
- [ ] Range validation for numeric arguments
- [ ] Format validation for string arguments

### IAM
- [ ] Specific resource ARNs (no wildcards)
- [ ] Condition keys used (region, account)
- [ ] Unused permissions removed

---

## Post-Deployment Verification

**Day 1:**
- [ ] Monitor CloudWatch alarms (no triggers)
- [ ] Review CloudWatch Logs for first 100 invocations
- [ ] Verify no tokens in logs
- [ ] Check DynamoDB encryption status
- [ ] Verify IAM role permissions

**Week 1:**
- [ ] Review security metrics (OAuth failures, SSRF attempts)
- [ ] Analyze CloudWatch Logs Insights queries
- [ ] Check Secrets Manager access patterns
- [ ] Verify secrets rotation scheduled

**Month 1:**
- [ ] Security audit of production logs
- [ ] Penetration testing (external)
- [ ] Review compliance status (GDPR, SOC 2)

---

## Emergency Contacts

**Security Incident:**
- Security Team: [Contact]
- On-Call DevOps: [Contact]

**Quick Remediation:**
```bash
# Disable Lambda immediately
aws lambda put-function-concurrency \
  --function-name rez-agent-webaction-prod \
  --reserved-concurrent-executions 0

# Rotate credentials immediately
aws secretsmanager rotate-secret \
  --secret-id rez-agent/golf/credentials
```

---

## Resources

- **Full Security Audit:** `/workspaces/rez_agent/docs/design/WEB_ACTION_SECURITY_AUDIT.md`
- **Summary:** `/workspaces/rez_agent/docs/design/SECURITY_AUDIT_SUMMARY.md`
- **Design Doc:** `/workspaces/rez_agent/docs/design/web-action-processor-design.md`

---

**Last Updated:** 2025-10-23
**Version:** 1.0
