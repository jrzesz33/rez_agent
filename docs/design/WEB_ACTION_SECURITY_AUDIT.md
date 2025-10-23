# Web Action Processor - Comprehensive Security Audit Report

**Document Version:** 1.0
**Date:** 2025-10-23
**Status:** Security Review Complete
**Auditor:** DevSecOps Security Specialist
**Scope:** Web Action Processor Feature (Design Phase)

---

## Executive Summary

This security audit examines the Web Action Processor feature designed for the rez_agent event-driven messaging system. The feature introduces HTTP integration capabilities to external APIs (weather.gov and birdsfoot.cps.golf) with OAuth 2.0 authentication, credential management via AWS Secrets Manager, and result storage in DynamoDB.

### Overall Security Posture: **MEDIUM RISK** (Requires Mitigation)

**Key Findings:**
- **Critical Issues:** 2 (Secrets Management, SSRF Prevention)
- **High Risk Issues:** 5 (OAuth Token Leakage, Input Validation, PII Handling, IAM Over-Permissions, Missing Secrets Rotation)
- **Medium Risk Issues:** 8
- **Low Risk Issues:** 6
- **Compliance Gaps:** 3 (GDPR PII, Audit Logging, Data Retention)

**Recommendation:** **Conditional Approval** - Implementation may proceed with mandatory implementation of all Critical and High risk mitigations before production deployment.

---

## Table of Contents

1. [Threat Model & Attack Surface Analysis](#1-threat-model--attack-surface-analysis)
2. [Risk Assessment Matrix](#2-risk-assessment-matrix)
3. [Vulnerability Analysis by Category](#3-vulnerability-analysis-by-category)
4. [IAM Security Review](#4-iam-security-review)
5. [Secrets Management Security](#5-secrets-management-security)
6. [Data Privacy & Compliance](#6-data-privacy--compliance)
7. [Network Security](#7-network-security)
8. [Input Validation & SSRF Prevention](#8-input-validation--ssrf-prevention)
9. [OAuth 2.0 Security](#9-oauth-20-security)
10. [Logging & Monitoring Security](#10-logging--monitoring-security)
11. [Security Testing Plan](#11-security-testing-plan)
12. [Security Metrics & KPIs](#12-security-metrics--kpis)
13. [Implementation Security Checklist](#13-implementation-security-checklist)
14. [Mitigation Roadmap](#14-mitigation-roadmap)

---

## 1. Threat Model & Attack Surface Analysis

### 1.1 STRIDE Threat Model

#### Spoofing
| Threat | Attack Vector | Impact | Likelihood | Risk |
|--------|--------------|--------|------------|------|
| **Credential Theft from Secrets Manager** | Attacker gains IAM credentials with `secretsmanager:GetSecretValue` permission | Full access to Golf API as legitimate user | Medium | **HIGH** |
| **Lambda Function Impersonation** | Attacker deploys malicious code via compromised CI/CD | Execute arbitrary HTTP requests with Lambda IAM role | Low | Medium |
| **OAuth Token Replay** | Intercepted access token reused before expiration | Unauthorized Golf API access for 60 minutes | Low | Medium |

#### Tampering
| Threat | Attack Vector | Impact | Likelihood | Risk |
|--------|--------------|--------|------------|------|
| **Message Payload Injection** | Modified DynamoDB message record with malicious URL | SSRF attack to internal AWS metadata service or internal networks | Medium | **CRITICAL** |
| **Lambda Environment Variable Tampering** | Attacker with Lambda update permissions modifies env vars | Redirect HTTP requests to attacker-controlled endpoints | Low | Medium |
| **DynamoDB Result Tampering** | Unauthorized modification of web action results | Data integrity compromise, fake notifications | Low | Low |

#### Repudiation
| Threat | Attack Vector | Impact | Likelihood | Risk |
|--------|--------------|--------|------------|------|
| **Insufficient Audit Logging** | OAuth authentication failures not logged with context | Cannot trace unauthorized access attempts | High | **HIGH** |
| **Missing Request Correlation** | HTTP requests not correlated to originating message | Difficult to attribute actions to source event | Medium | Medium |

#### Information Disclosure
| Threat | Attack Vector | Impact | Likelihood | Risk |
|--------|--------------|--------|------------|------|
| **OAuth Token Leakage in Logs** | Access tokens logged in error messages or debug logs | Token compromise allows 60 minutes of unauthorized access | High | **CRITICAL** |
| **PII Exposure in DynamoDB** | Golf reservation data contains personal information stored for 3 days | GDPR/privacy violation, potential data breach | High | **HIGH** |
| **Secrets in CloudWatch Logs** | Password accidentally logged during debugging | Permanent credential exposure in log retention period | Medium | **HIGH** |
| **HTTP Response Bodies Stored Unencrypted** | Golf API responses may contain sensitive data (full names, email) | Compliance violation, privacy risk | High | **HIGH** |

#### Denial of Service
| Threat | Attack Vector | Impact | Likelihood | Risk |
|--------|--------------|--------|------------|------|
| **SSRF-Based Resource Exhaustion** | Malicious payload forces Lambda to connect to slow/unresponsive endpoints | Lambda timeout, cost increase, service degradation | Medium | Medium |
| **Secrets Manager Rate Limiting** | Excessive secret retrieval calls exceed service quotas | Lambda failures, cascading errors | Low | Low |
| **DynamoDB Write Throttling** | Result storage exceeds provisioned capacity | Message processing failures, backlog | Low | Low |

#### Elevation of Privilege
| Threat | Attack Vector | Impact | Likelihood | Risk |
|--------|--------------|--------|------------|------|
| **IAM Role Over-Permissions** | Lambda role has broader permissions than necessary | Lateral movement, privilege escalation | Medium | **HIGH** |
| **Secrets Manager Wildcard Access** | `arn:aws:secretsmanager:*:*:secret:rez-agent/*` allows access to all secrets | Access to unrelated credentials (if future secrets added) | Medium | Medium |

### 1.2 Attack Surface Map

```
External Attack Surface:
┌─────────────────────────────────────────────────────────────┐
│ 1. Weather.gov API (No Auth)                               │
│    - Risk: Limited (read-only, public data)                │
│    - Mitigation: URL validation, response size limits      │
└─────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│ 2. Birdsfoot Golf API (OAuth 2.0)                          │
│    - Risk: HIGH (PII, authentication required)             │
│    - Attack: Credential theft, token replay                │
│    - Mitigation: Secrets rotation, token validation        │
└─────────────────────────────────────────────────────────────┘

Internal Attack Surface:
┌─────────────────────────────────────────────────────────────┐
│ 3. DynamoDB Message Table                                  │
│    - Risk: CRITICAL (SSRF if payload tampered)             │
│    - Attack: Malicious URL injection                       │
│    - Mitigation: URL allowlist, payload validation         │
└─────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│ 4. AWS Secrets Manager                                     │
│    - Risk: HIGH (credential exposure)                      │
│    - Attack: IAM privilege escalation, log exposure        │
│    - Mitigation: Least privilege IAM, rotation, encryption │
└─────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│ 5. Lambda Execution Environment                            │
│    - Risk: MEDIUM (environment variable tampering)         │
│    - Attack: Code injection via CI/CD compromise           │
│    - Mitigation: Code signing, deployment approvals        │
└─────────────────────────────────────────────────────────────┘
```

### 1.3 Trust Boundaries

```
Trust Boundary 1: External → Internal
- Weather.gov API → Lambda (HTTP/TLS)
  Risk: Response injection, malformed data

- Golf API → Lambda (HTTP/TLS + OAuth)
  Risk: Token theft, MitM if TLS misconfigured

Trust Boundary 2: Internal Services
- DynamoDB → Lambda
  Risk: Malicious payload stored upstream

- Secrets Manager → Lambda
  Risk: IAM permission abuse

Trust Boundary 3: Lambda → External
- Lambda → External APIs
  Risk: SSRF, credential leakage in requests
```

---

## 2. Risk Assessment Matrix

### 2.1 Risk Scoring Criteria

**Likelihood Scale:**
- Low (1): Requires significant effort, rare scenario
- Medium (2): Moderate effort, plausible scenario
- High (3): Easy to exploit, common scenario

**Impact Scale:**
- Low (1): Minimal damage, easily recoverable
- Medium (2): Moderate damage, service degradation
- High (3): Severe damage, data breach, compliance violation
- Critical (4): Catastrophic damage, system compromise, legal liability

**Risk Score = Likelihood × Impact**

### 2.2 Prioritized Risk Register

| ID | Vulnerability | Likelihood | Impact | Risk Score | Priority |
|----|--------------|------------|--------|------------|----------|
| **V-001** | SSRF via URL injection in web_action payload | Medium (2) | Critical (4) | **8** | **P0 - Critical** |
| **V-002** | OAuth token leakage in CloudWatch Logs | High (3) | High (3) | **9** | **P0 - Critical** |
| **V-003** | PII exposure in DynamoDB results (no encryption) | High (3) | High (3) | **9** | **P1 - High** |
| **V-004** | Secrets Manager credentials logged in error messages | Medium (2) | High (3) | **6** | **P1 - High** |
| **V-005** | IAM role over-permissions (wildcard Secrets Manager access) | Medium (2) | High (3) | **6** | **P1 - High** |
| **V-006** | Missing secrets rotation mechanism | High (3) | Medium (2) | **6** | **P1 - High** |
| **V-007** | Insufficient audit logging for OAuth failures | High (3) | Medium (2) | **6** | **P1 - High** |
| **V-008** | HTTP client missing certificate pinning | Medium (2) | Medium (2) | **4** | **P2 - Medium** |
| **V-009** | Token cache persists across Lambda invocations | Medium (2) | Medium (2) | **4** | **P2 - Medium** |
| **V-010** | Missing input validation for action arguments | Medium (2) | Medium (2) | **4** | **P2 - Medium** |
| **V-011** | DynamoDB results table missing encryption at rest | Low (1) | High (3) | **3** | **P2 - Medium** |
| **V-012** | CloudWatch Logs retention exceeds data minimization | Medium (2) | Low (1) | **2** | **P3 - Low** |
| **V-013** | Missing X-Ray tracing for security events | Low (1) | Low (1) | **1** | **P3 - Low** |

---

## 3. Vulnerability Analysis by Category

### 3.1 CRITICAL VULNERABILITIES

#### **V-001: Server-Side Request Forgery (SSRF) via URL Injection**

**Severity:** CRITICAL (CVSS 9.8)
**CWE:** CWE-918 (Server-Side Request Forgery)

**Description:**
The design accepts a `url` field from the `WebActionPayload` stored in DynamoDB. If an attacker can modify the DynamoDB message record (via compromised Scheduler Lambda, Web API, or direct DynamoDB access), they can inject arbitrary URLs:

```json
{
  "url": "http://169.254.169.254/latest/meta-data/iam/security-credentials/rez-agent-processor-role",
  "action": "fetch_weather"
}
```

**Attack Scenario:**
1. Attacker gains write access to DynamoDB messages table
2. Modifies existing message or creates new message with malicious URL
3. Web Action Processor Lambda executes HTTP GET to AWS metadata service
4. IAM credentials exfiltrated in response body, stored in DynamoDB results
5. Attacker retrieves credentials from results table

**Potential Impact:**
- **AWS Account Takeover:** IAM role credentials leaked
- **Internal Network Scanning:** Map internal VPC resources (if Lambda in VPC)
- **Data Exfiltration:** Access internal services via SSRF
- **Cloud Provider Metadata Exposure:** EC2 metadata, ECS task metadata

**Current Mitigation (Design):**
- None explicitly documented

**Required Mitigations:**
1. **URL Allowlist (MANDATORY):**
   ```go
   var allowedHosts = map[string]bool{
       "api.weather.gov":              true,
       "birdsfoot.cps.golf":           true,
   }

   func validateURL(rawURL string) error {
       parsed, err := url.Parse(rawURL)
       if err != nil {
           return fmt.Errorf("invalid URL: %w", err)
       }

       // Block private IP ranges
       if isPrivateIP(parsed.Host) {
           return fmt.Errorf("private IP addresses not allowed")
       }

       // Block AWS metadata endpoints
       if parsed.Host == "169.254.169.254" || parsed.Host == "fd00:ec2::254" {
           return fmt.Errorf("AWS metadata endpoint blocked")
       }

       // Check allowlist
       if !allowedHosts[parsed.Hostname()] {
           return fmt.Errorf("host not in allowlist: %s", parsed.Hostname())
       }

       // Require HTTPS
       if parsed.Scheme != "https" {
           return fmt.Errorf("only HTTPS allowed")
       }

       return nil
   }
   ```

2. **IP Address Validation:**
   - Block private ranges: 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16
   - Block localhost: 127.0.0.0/8, ::1
   - Block link-local: 169.254.0.0/16, fe80::/10
   - Block metadata: 169.254.169.254, fd00:ec2::254

3. **DNS Rebinding Protection:**
   - Validate hostname, then resolve to IP
   - Re-validate IP before HTTP request
   - Prevent DNS TOCTOU attacks

4. **HTTP Client Restrictions:**
   - Disable redirects or limit to 2 max
   - Set restrictive timeout (30s)
   - Validate response Content-Type

#### **V-002: OAuth Token Leakage in Logs**

**Severity:** CRITICAL (CVSS 8.5)
**CWE:** CWE-532 (Insertion of Sensitive Information into Log File)

**Description:**
The design documentation shows OAuth token handling but lacks explicit redaction requirements. Common logging mistakes:

```go
// VULNERABLE CODE (Example of what NOT to do)
logger.InfoContext(ctx, "OAuth request",
    slog.String("token", accessToken))  // Token logged!

logger.ErrorContext(ctx, "HTTP request failed",
    slog.String("authorization_header", req.Header.Get("Authorization"))) // Token in header!
```

**Attack Scenario:**
1. Developer logs OAuth token for debugging during development
2. Code deployed to production without removing debug logs
3. Access token appears in CloudWatch Logs
4. Attacker with CloudWatch Logs read access retrieves token
5. Token valid for 60 minutes, used to access Golf API

**Potential Impact:**
- **Unauthorized API Access:** 60-minute window of Golf API access
- **PII Exposure:** Access to all golf reservations
- **Compliance Violation:** Credential exposure violates PCI-DSS, SOC 2

**Current Mitigation (Design):**
- Documentation states "OAuth tokens never logged or persisted" (line 103)
- No enforcement mechanism described

**Required Mitigations:**
1. **Structured Logging Redaction:**
   ```go
   // Create redacted logger wrapper
   type RedactedLogger struct {
       logger *slog.Logger
   }

   func (l *RedactedLogger) LogHTTPRequest(ctx context.Context, req *http.Request) {
       headers := make(map[string]string)
       for k, v := range req.Header {
           if k == "Authorization" {
               headers[k] = "[REDACTED]"
           } else {
               headers[k] = strings.Join(v, ",")
           }
       }

       l.logger.InfoContext(ctx, "HTTP request",
           slog.String("method", req.Method),
           slog.String("url", redactSensitiveParams(req.URL.String())),
           slog.Any("headers", headers),
       )
   }
   ```

2. **Static Analysis Enforcement:**
   - Implement linter rule to detect token logging patterns
   - Example: `grep -r "slog.*token\|slog.*password\|slog.*secret"`

3. **CloudWatch Logs Insights Query for Detection:**
   ```
   fields @timestamp, @message
   | filter @message like /Bearer\s+[A-Za-z0-9\-_]+\.[A-Za-z0-9\-_]+\.[A-Za-z0-9\-_]+/
   | stats count() as token_leaks
   ```

4. **Lambda Environment Variable Protection:**
   - Never log environment variables wholesale
   - Redact `NTFY_URL`, `SNS_TOPIC_ARN` if they contain tokens

---

### 3.2 HIGH RISK VULNERABILITIES

#### **V-003: PII Exposure in DynamoDB Results**

**Severity:** HIGH (CVSS 7.5)
**CWE:** CWE-311 (Missing Encryption of Sensitive Data)

**Description:**
Golf reservation API responses contain PII:
- Full names
- Email addresses (possibly)
- Phone numbers (possibly)
- Tee time reservations (behavioral data)

Design stores raw API response in `response_body` field for 3 days with only TTL-based deletion.

**Compliance Impact:**
- **GDPR Article 32:** Requires encryption of personal data
- **GDPR Article 17:** Right to erasure - 3-day retention may be excessive
- **CCPA:** Personal information must be protected

**Current Mitigation:**
- 3-day TTL limits exposure window
- No encryption at rest mentioned

**Required Mitigations:**

1. **DynamoDB Encryption at Rest (MANDATORY):**
   ```go
   // Pulumi infrastructure update
   resultsTable, err := dynamodb.NewTable(ctx, "web-action-results", &dynamodb.TableArgs{
       // ... existing config ...
       ServerSideEncryption: &dynamodb.TableServerSideEncryptionArgs{
           Enabled: pulumi.Bool(true),
           KmsKeyArn: kmsKey.Arn, // Customer-managed KMS key
       },
   })
   ```

2. **Field-Level Encryption for PII:**
   ```go
   // Encrypt sensitive fields before storage
   func encryptPII(plaintext string, kmsClient *kms.Client) (string, error) {
       result, err := kmsClient.Encrypt(context.TODO(), &kms.EncryptInput{
           KeyId:     aws.String("alias/rez-agent-pii"),
           Plaintext: []byte(plaintext),
       })
       if err != nil {
           return "", err
       }
       return base64.StdEncoding.EncodeToString(result.CiphertextBlob), nil
   }
   ```

3. **Data Minimization:**
   - Only store necessary fields from response
   - Hash or pseudonymize user identifiers
   - Reduce TTL to 24 hours if acceptable

4. **Access Logging:**
   - Enable CloudTrail data events for DynamoDB table
   - Alert on unusual access patterns

#### **V-004: Secrets in Error Messages**

**Severity:** HIGH (CVSS 7.0)
**CWE:** CWE-209 (Generation of Error Message Containing Sensitive Information)

**Description:**
Error handling may inadvertently log credentials:

```go
// VULNERABLE
creds, err := secretsCache.GetGolfCredentials(ctx)
if err != nil {
    return fmt.Errorf("failed to get credentials: %w", err) // May expose secret value
}
```

**Required Mitigations:**
1. **Sanitize Error Messages:**
   ```go
   func sanitizeSecretError(err error) error {
       if err == nil {
           return nil
       }
       // Remove any secret values from error message
       msg := err.Error()
       msg = regexp.MustCompile(`"password":"[^"]*"`).ReplaceAllString(msg, `"password":"[REDACTED]"`)
       msg = regexp.MustCompile(`"username":"[^"]*"`).ReplaceAllString(msg, `"username":"[REDACTED]"`)
       return errors.New(msg)
   }
   ```

2. **Error Wrapping Best Practices:**
   - Never wrap errors that contain secret values
   - Return generic error messages to logs
   - Use error codes instead of detailed messages

#### **V-005: IAM Role Over-Permissions**

**Severity:** HIGH (CVSS 6.8)
**CWE:** CWE-269 (Improper Privilege Management)

**Description:**
Proposed IAM policy uses wildcards:

```json
{
  "Effect": "Allow",
  "Action": "secretsmanager:GetSecretValue",
  "Resource": "arn:aws:secretsmanager:*:*:secret:rez-agent/*"
}
```

**Issues:**
- Allows access to ANY secret under `rez-agent/` prefix
- Future secrets automatically accessible
- No region/account restrictions

**Required Mitigations:**

1. **Principle of Least Privilege:**
   ```json
   {
     "Effect": "Allow",
     "Action": "secretsmanager:GetSecretValue",
     "Resource": [
       "arn:aws:secretsmanager:us-east-1:ACCOUNT_ID:secret:rez-agent/golf/credentials-*"
     ],
     "Condition": {
       "StringEquals": {
         "aws:RequestedRegion": "us-east-1"
       }
     }
   }
   ```

2. **Remove Unused Permissions:**
   - Current design shows `dynamodb:Query` on messages table - only needs `GetItem`
   - Remove `dynamodb:UpdateItem` on results table if not used

3. **Time-Based Restrictions (Optional):**
   ```json
   "Condition": {
     "DateGreaterThan": {"aws:CurrentTime": "2025-10-23T09:00:00Z"},
     "DateLessThan": {"aws:CurrentTime": "2025-10-23T12:00:00Z"}
   }
   ```

#### **V-006: Missing Secrets Rotation**

**Severity:** HIGH (CVSS 6.5)
**CWE:** CWE-798 (Use of Hard-coded Credentials)

**Description:**
Design states "Rotation: Manual (initially)" with no automated rotation plan.

**Risks:**
- Credentials valid indefinitely
- No detection of compromised credentials
- Compliance violations (PCI-DSS 8.2.4 requires 90-day rotation)

**Required Mitigations:**

1. **Secrets Manager Automatic Rotation:**
   ```go
   // Pulumi configuration
   golfSecret, err := secretsmanager.NewSecret(ctx, "golf-credentials", &secretsmanager.SecretArgs{
       Name: pulumi.String("rez-agent/golf/credentials"),
       RotationRules: &secretsmanager.SecretRotationRulesArgs{
           AutomaticallyAfterDays: pulumi.Int(90),
       },
   })

   // Rotation Lambda
   _, err = secretsmanager.NewSecretRotation(ctx, "golf-rotation", &secretsmanager.SecretRotationArgs{
       SecretId: golfSecret.ID(),
       RotationLambdaArn: rotationLambda.Arn,
       RotationRules: &secretsmanager.SecretRotationRotationRulesArgs{
           AutomaticallyAfterDays: pulumi.Int(90),
       },
   })
   ```

2. **Rotation Lambda Implementation:**
   - Create new Golf API credentials
   - Test new credentials
   - Update Secrets Manager
   - Deprecate old credentials after grace period

3. **Rotation Monitoring:**
   - CloudWatch alarm if rotation fails
   - SNS notification to security team

#### **V-007: Insufficient Audit Logging**

**Severity:** HIGH (CVSS 6.2)
**CWE:** CWE-778 (Insufficient Logging)

**Description:**
Security-relevant events not explicitly logged:
- OAuth authentication attempts (success/failure)
- Secrets Manager access
- URL validation failures
- Unauthorized action attempts

**Compliance Impact:**
- **SOC 2 CC7.2:** Requires logging of security events
- **PCI-DSS 10.2:** Requires audit logs for authentication
- **GDPR Article 30:** Records of processing activities

**Required Mitigations:**

1. **Security Event Logging Standard:**
   ```go
   type SecurityEvent struct {
       EventType    string    `json:"event_type"`
       EventTime    time.Time `json:"event_time"`
       Principal    string    `json:"principal"`
       Action       string    `json:"action"`
       Resource     string    `json:"resource"`
       Result       string    `json:"result"` // success, failure, denied
       SourceIP     string    `json:"source_ip,omitempty"`
       UserAgent    string    `json:"user_agent,omitempty"`
       ErrorCode    string    `json:"error_code,omitempty"`
       CorrelationID string   `json:"correlation_id"`
   }

   func logSecurityEvent(ctx context.Context, event SecurityEvent) {
       logger.InfoContext(ctx, "SECURITY_EVENT",
           slog.String("event_type", event.EventType),
           slog.String("action", event.Action),
           slog.String("result", event.Result),
           slog.String("correlation_id", event.CorrelationID),
       )
   }
   ```

2. **Required Security Events:**
   - `oauth_authentication_attempt` (success/failure)
   - `secret_accessed` (which secret, by which Lambda)
   - `url_validation_failed` (attempted URL, reason)
   - `http_request_executed` (destination, status code)
   - `action_handler_invoked` (action type, duration)

3. **CloudWatch Logs Insights Dashboard:**
   ```
   fields @timestamp, event_type, action, result
   | filter event_type = "oauth_authentication_attempt" and result = "failure"
   | stats count() as failures by bin(5m)
   ```

---

### 3.3 MEDIUM RISK VULNERABILITIES

#### **V-008: Missing Certificate Pinning**

**Severity:** MEDIUM (CVSS 5.9)
**CWE:** CWE-295 (Improper Certificate Validation)

**Description:**
HTTP client uses standard TLS verification without certificate pinning. Vulnerable to MitM if attacker controls DNS or has CA-signed certificate.

**Mitigation:**
```go
// Certificate pinning for critical APIs
var golfAPICertFingerprint = "sha256/AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="

tlsConfig := &tls.Config{
    MinVersion: tls.VersionTLS12,
    VerifyPeerCertificate: func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
        for _, rawCert := range rawCerts {
            cert, _ := x509.ParseCertificate(rawCert)
            fingerprint := sha256.Sum256(cert.Raw)
            if base64.StdEncoding.EncodeToString(fingerprint[:]) == golfAPICertFingerprint {
                return nil
            }
        }
        return errors.New("certificate pinning failed")
    },
}
```

#### **V-009: OAuth Token Cache Security**

**Severity:** MEDIUM (CVSS 5.3)

**Description:**
Design caches OAuth tokens in Lambda global scope for 50 minutes. Risks:
- Token persists across invocations in same execution environment
- No encryption of cached token in memory
- Lambda environment reused by different executions

**Mitigations:**
1. **In-Memory Encryption:**
   ```go
   type SecureTokenCache struct {
       encryptedToken []byte
       expiresAt      time.Time
       mu             sync.RWMutex
   }

   func (c *SecureTokenCache) SetToken(token string, ttl time.Duration) error {
       encrypted, err := encryptInMemory([]byte(token))
       if err != nil {
           return err
       }
       c.mu.Lock()
       defer c.mu.Unlock()
       c.encryptedToken = encrypted
       c.expiresAt = time.Now().Add(ttl)
       return nil
   }
   ```

2. **Cache Invalidation:**
   - Clear cache on OAuth failures
   - Shorten cache TTL to 45 minutes (5-minute buffer)

#### **V-010: Input Validation Gaps**

**Severity:** MEDIUM (CVSS 5.0)

**Description:**
WebActionPayload arguments not validated:
```json
{
  "arguments": {
    "days": -1000,  // Negative value
    "max_results": 999999999,  // Excessive value
    "golfer_id": "<script>alert(1)</script>"  // Injection attempt
  }
}
```

**Mitigations:**
```go
func (p *WebActionPayload) Validate() error {
    // ... existing validation ...

    if days, ok := p.Arguments["days"].(float64); ok {
        if days < 1 || days > 14 {
            return fmt.Errorf("days must be between 1 and 14")
        }
    }

    if maxResults, ok := p.Arguments["max_results"].(float64); ok {
        if maxResults < 1 || maxResults > 100 {
            return fmt.Errorf("max_results must be between 1 and 100")
        }
    }

    if golferID, ok := p.Arguments["golfer_id"].(string); ok {
        if !regexp.MustCompile(`^[0-9]+$`).MatchString(golferID) {
            return fmt.Errorf("golfer_id must be numeric")
        }
    }

    return nil
}
```

---

## 4. IAM Security Review

### 4.1 Current IAM Policy Analysis

**Proposed Policy (from design doc lines 926-981):**

**FINDINGS:**

1. **Over-Permissive Secrets Access:**
   ```json
   "Resource": "arn:aws:secretsmanager:*:*:secret:rez-agent/*"
   ```
   - Allows access to ALL secrets with `rez-agent/` prefix
   - No region restriction
   - Violates principle of least privilege

2. **Unnecessary DynamoDB Permissions:**
   - `dynamodb:Query` on messages table likely unnecessary (only needs `GetItem`)
   - `dynamodb:UpdateItem` on results table not used in design

3. **Wildcard CloudWatch Logs:**
   ```json
   "Resource": "arn:aws:logs:*:*:*"
   ```
   - Too broad, should be scoped to function's log group

4. **Missing Condition Keys:**
   - No IP address restrictions
   - No time-based restrictions
   - No MFA requirements

### 4.2 Recommended IAM Policy

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "DynamoDBMessagesReadOnly",
      "Effect": "Allow",
      "Action": [
        "dynamodb:GetItem"
      ],
      "Resource": [
        "arn:aws:dynamodb:us-east-1:ACCOUNT_ID:table/rez-agent-messages-${stage}"
      ],
      "Condition": {
        "StringEquals": {
          "dynamodb:LeadingKeys": ["${aws:username}"]
        }
      }
    },
    {
      "Sid": "DynamoDBResultsWrite",
      "Effect": "Allow",
      "Action": [
        "dynamodb:PutItem"
      ],
      "Resource": [
        "arn:aws:dynamodb:us-east-1:ACCOUNT_ID:table/rez-agent-web-action-results-${stage}"
      ]
    },
    {
      "Sid": "SQSMessageConsumer",
      "Effect": "Allow",
      "Action": [
        "sqs:ReceiveMessage",
        "sqs:DeleteMessage",
        "sqs:GetQueueAttributes"
      ],
      "Resource": "arn:aws:sqs:us-east-1:ACCOUNT_ID:rez-agent-web-actions-${stage}"
    },
    {
      "Sid": "SNSPublishCompletion",
      "Effect": "Allow",
      "Action": "sns:Publish",
      "Resource": "arn:aws:sns:us-east-1:ACCOUNT_ID:rez-agent-messages-${stage}",
      "Condition": {
        "StringEquals": {
          "sns:Protocol": "sqs"
        }
      }
    },
    {
      "Sid": "SecretsManagerGolfCredentials",
      "Effect": "Allow",
      "Action": "secretsmanager:GetSecretValue",
      "Resource": "arn:aws:secretsmanager:us-east-1:ACCOUNT_ID:secret:rez-agent/golf/credentials-??????",
      "Condition": {
        "StringEquals": {
          "aws:RequestedRegion": "us-east-1"
        }
      }
    },
    {
      "Sid": "CloudWatchLogsWrite",
      "Effect": "Allow",
      "Action": [
        "logs:CreateLogStream",
        "logs:PutLogEvents"
      ],
      "Resource": "arn:aws:logs:us-east-1:ACCOUNT_ID:log-group:/aws/lambda/rez-agent-webaction-${stage}:*"
    },
    {
      "Sid": "XRayTracing",
      "Effect": "Allow",
      "Action": [
        "xray:PutTraceSegments",
        "xray:PutTelemetryRecords"
      ],
      "Resource": "*"
    },
    {
      "Sid": "DenyDangerousActions",
      "Effect": "Deny",
      "Action": [
        "iam:*",
        "secretsmanager:PutSecretValue",
        "secretsmanager:DeleteSecret",
        "kms:Decrypt"
      ],
      "Resource": "*"
    }
  ]
}
```

### 4.3 IAM Policy Testing

**Test Cases:**
1. Verify Lambda can read specific Golf credentials secret
2. Verify Lambda CANNOT read other secrets (e.g., `rez-agent/other/secret`)
3. Verify Lambda can write to results table
4. Verify Lambda CANNOT modify IAM policies
5. Verify Lambda CANNOT decrypt KMS keys directly

---

## 5. Secrets Management Security

### 5.1 Current Design Analysis

**Secrets Manager Secret: `rez-agent/golf/credentials`**

```json
{
  "username": "user@example.com",
  "password": "SecurePassword123!",
  "golfer_id": "91124"
}
```

**Vulnerabilities:**
1. No rotation strategy (manual only)
2. No versioning management
3. No audit logging of secret access
4. Credentials cached for Lambda lifecycle (unbounded)

### 5.2 Secrets Management Best Practices

#### 5.2.1 Secret Rotation Strategy

**Automatic Rotation Schedule:**
- Golf API credentials: 90 days
- Emergency rotation: On-demand via API

**Rotation Lambda Pseudocode:**
```go
func RotateGolfCredentials(ctx context.Context, event SecretsManagerRotationEvent) error {
    switch event.Step {
    case "createSecret":
        // Generate new password, update Golf API
        newPassword := generateSecurePassword(32)
        err := golfAPIClient.UpdatePassword(currentCreds.Username, newPassword)
        if err != nil {
            return err
        }
        // Store new version in Secrets Manager
        newSecret := GolfCredentials{
            Username: currentCreds.Username,
            Password: newPassword,
            GolferID: currentCreds.GolferID,
        }
        return secretsClient.PutSecretValue(secretARN, newSecret, "AWSPENDING")

    case "setSecret":
        // Test new credentials
        testCreds := getSecretVersion(secretARN, "AWSPENDING")
        token, err := authenticateWithGolfAPI(testCreds)
        if err != nil {
            return fmt.Errorf("new credentials failed: %w", err)
        }
        return nil

    case "testSecret":
        // Validation already done in setSecret
        return nil

    case "finishSecret":
        // Move AWSPENDING to AWSCURRENT
        return secretsClient.UpdateVersionStage(secretARN, "AWSCURRENT", "AWSPENDING")
    }
    return nil
}
```

#### 5.2.2 Secret Access Logging

**Enable CloudTrail Data Events:**
```json
{
  "eventSelectors": [
    {
      "readWriteType": "All",
      "includeManagementEvents": true,
      "dataResources": [
        {
          "type": "AWS::SecretsManager::Secret",
          "values": ["arn:aws:secretsmanager:*:*:secret:rez-agent/*"]
        }
      ]
    }
  ]
}
```

**CloudWatch Alarm for Unusual Access:**
```
fields @timestamp, userIdentity.principalId, requestParameters.secretId
| filter eventName = "GetSecretValue" and requestParameters.secretId like /rez-agent/
| stats count() as access_count by userIdentity.principalId
| filter access_count > 1000
```

#### 5.2.3 Secret Encryption

**Current:** AWS-managed KMS key
**Recommended:** Customer-managed KMS key with rotation

```go
// Pulumi KMS key for secrets
kmsKey, err := kms.NewKey(ctx, "rez-agent-secrets-key", &kms.KeyArgs{
    Description:           pulumi.String("Encryption key for rez-agent secrets"),
    EnableKeyRotation:     pulumi.Bool(true),
    DeletionWindowInDays:  pulumi.Int(30),
    Tags: commonTags,
})

// Secret with customer-managed key
golfSecret, err := secretsmanager.NewSecret(ctx, "golf-credentials", &secretsmanager.SecretArgs{
    Name:      pulumi.String("rez-agent/golf/credentials"),
    KmsKeyId:  kmsKey.ID(),
})
```

**Benefits:**
- Audit trail of key usage
- Ability to disable key in emergency
- Compliance with data sovereignty requirements

---

## 6. Data Privacy & Compliance

### 6.1 GDPR Compliance Assessment

#### 6.1.1 Personal Data Inventory

**Golf Reservations Data:**
| Data Element | Classification | Retention | Legal Basis |
|--------------|----------------|-----------|-------------|
| Reservation confirmation number | PII | 3 days | Legitimate interest |
| Golf course name | Non-PII | 3 days | N/A |
| Number of players | Potentially PII | 3 days | Legitimate interest |
| Tee time date/time | Behavioral data | 3 days | Legitimate interest |
| User's reservations (aggregate) | PII | 3 days | Legitimate interest |

**GDPR Article Compliance:**
- **Article 5(1)(e) - Storage Limitation:** 3-day TTL acceptable if justified
- **Article 25 - Data Protection by Design:** Requires encryption (currently MISSING)
- **Article 32 - Security of Processing:** Requires encryption, pseudonymization
- **Article 30 - Records of Processing:** Requires audit logs (partially implemented)

#### 6.1.2 GDPR Compliance Gaps

| Requirement | Status | Gap | Mitigation |
|-------------|--------|-----|------------|
| Encryption at rest (Art 32) | ❌ MISSING | DynamoDB results table not encrypted | Enable DynamoDB encryption with KMS |
| Data minimization (Art 5) | ⚠️ PARTIAL | Stores full API response | Only store necessary fields |
| Right to erasure (Art 17) | ❌ MISSING | No manual deletion mechanism | Implement delete API endpoint |
| Data breach notification (Art 33) | ⚠️ PARTIAL | No automated breach detection | CloudWatch alarm for data exfiltration |
| Privacy by design (Art 25) | ⚠️ PARTIAL | PII not pseudonymized | Hash user identifiers |

#### 6.1.3 Recommended Data Protection Measures

**1. Pseudonymization:**
```go
func pseudonymizeReservation(reservation GolfReservation) GolfReservation {
    hasher := sha256.New()
    hasher.Write([]byte(reservation.ConfirmationNumber))
    pseudonym := hex.EncodeToString(hasher.Sum(nil))[:16]

    return GolfReservation{
        ConfirmationNumber: pseudonym,  // Hashed
        DateTime:           reservation.DateTime,
        CourseName:         reservation.CourseName,
        NumberOfPlayers:    0,  // Aggregated, not stored
    }
}
```

**2. Data Retention Policy:**
```go
// Reduce TTL to 24 hours instead of 3 days
func NewWebActionResult(messageID, action, url string, stage Stage) *WebActionResult {
    now := time.Now().UTC()
    ttl := now.Add(24 * time.Hour).Unix()  // Changed from 3 days
    // ...
}
```

**3. Right to Erasure API:**
```go
func DeleteUserData(ctx context.Context, userID string) error {
    // Query all results containing user's data
    results, err := queryResultsByUser(ctx, userID)
    for _, result := range results {
        err := dynamoClient.DeleteItem(ctx, &dynamodb.DeleteItemInput{
            TableName: aws.String(resultsTableName),
            Key: map[string]types.AttributeValue{
                "action_id": &types.AttributeValueMemberS{Value: result.ActionID},
            },
        })
    }
    return nil
}
```

### 6.2 Compliance Checklist

#### SOC 2 Type II

| Control | Requirement | Status | Evidence |
|---------|-------------|--------|----------|
| CC6.1 - Logical Access | Least privilege IAM | ⚠️ PARTIAL | IAM policies over-permissive |
| CC6.6 - Encryption | Encrypt data at rest | ❌ MISSING | No DynamoDB encryption |
| CC7.2 - Security Monitoring | Log security events | ⚠️ PARTIAL | OAuth failures not logged |
| CC7.3 - Incident Response | Detect anomalies | ⚠️ PARTIAL | No automated alerts |

#### PCI-DSS (if payment data involved)

| Requirement | Status | Notes |
|-------------|--------|-------|
| 3.4 - Render PAN unreadable | ✅ N/A | No payment card data |
| 8.2.4 - Change passwords every 90 days | ❌ MISSING | No credential rotation |
| 10.2 - Audit logs | ⚠️ PARTIAL | Incomplete logging |

---

## 7. Network Security

### 7.1 TLS Configuration Review

**Current Design (lines 1089-1115):**
```go
TLSClientConfig: &tls.Config{
    MinVersion: tls.VersionTLS12,
    MaxVersion: tls.VersionTLS13,
}
```

**Security Assessment:** ✅ ACCEPTABLE

**Recommendations:**
1. **Disable TLS 1.2 for production:**
   ```go
   MinVersion: tls.VersionTLS13,  // TLS 1.3 only
   ```

2. **Configure Cipher Suites (TLS 1.2 fallback):**
   ```go
   CipherSuites: []uint16{
       tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
       tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
   },
   ```

3. **Enable OCSP Stapling:**
   ```go
   tlsConfig.PreferServerCipherSuites = true
   ```

### 7.2 HTTP Client Security

**Vulnerabilities:**
1. No redirect following limits (default: 10)
2. No response size limits
3. No connection pooling limits

**Hardened HTTP Client:**
```go
func NewSecureHTTPClient(timeout time.Duration) *http.Client {
    transport := &http.Transport{
        TLSClientConfig: &tls.Config{
            MinVersion: tls.VersionTLS13,
            MaxVersion: tls.VersionTLS13,
        },
        MaxIdleConns:        10,
        MaxIdleConnsPerHost: 2,
        IdleConnTimeout:     90 * time.Second,
        DisableKeepAlives:   false,

        // DNS security
        DialContext: (&net.Dialer{
            Timeout:   30 * time.Second,
            KeepAlive: 30 * time.Second,
            Resolver: &net.Resolver{
                PreferGo: true,  // Use Go DNS resolver
                Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
                    // Use CloudFlare DNS for DNS-over-HTTPS
                    return net.Dial(network, "1.1.1.1:53")
                },
            },
        }).DialContext,
    }

    return &http.Client{
        Timeout:   timeout,
        Transport: transport,
        CheckRedirect: func(req *http.Request, via []*http.Request) error {
            if len(via) >= 2 {  // Max 2 redirects
                return fmt.Errorf("too many redirects")
            }
            // Validate redirect URL
            if err := validateURL(req.URL.String()); err != nil {
                return fmt.Errorf("invalid redirect: %w", err)
            }
            return nil
        },
    }
}
```

### 7.3 Response Size Limits

**Prevent Memory Exhaustion:**
```go
func readResponse(resp *http.Response) ([]byte, error) {
    // Limit response size to 10MB
    limitedReader := io.LimitReader(resp.Body, 10*1024*1024)
    body, err := io.ReadAll(limitedReader)
    if err != nil {
        return nil, fmt.Errorf("failed to read response: %w", err)
    }

    // Check if response was truncated
    if len(body) == 10*1024*1024 {
        return nil, fmt.Errorf("response too large (>10MB)")
    }

    return body, nil
}
```

---

## 8. Input Validation & SSRF Prevention

### 8.1 Comprehensive URL Validation

**Multi-Layer Validation:**

```go
package validation

import (
    "fmt"
    "net"
    "net/url"
    "regexp"
    "strings"
)

// URLValidator provides comprehensive URL validation
type URLValidator struct {
    allowedHosts map[string]bool
    allowedSchemes map[string]bool
}

func NewURLValidator() *URLValidator {
    return &URLValidator{
        allowedHosts: map[string]bool{
            "api.weather.gov":     true,
            "birdsfoot.cps.golf":  true,
        },
        allowedSchemes: map[string]bool{
            "https": true,
        },
    }
}

// Validate performs comprehensive URL validation
func (v *URLValidator) Validate(rawURL string) error {
    // Step 1: Parse URL
    parsed, err := url.Parse(rawURL)
    if err != nil {
        return fmt.Errorf("invalid URL syntax: %w", err)
    }

    // Step 2: Validate scheme
    if !v.allowedSchemes[parsed.Scheme] {
        return fmt.Errorf("disallowed scheme: %s (only HTTPS allowed)", parsed.Scheme)
    }

    // Step 3: Extract hostname
    hostname := parsed.Hostname()
    if hostname == "" {
        return fmt.Errorf("missing hostname")
    }

    // Step 4: Check allowlist
    if !v.allowedHosts[hostname] {
        return fmt.Errorf("host not in allowlist: %s", hostname)
    }

    // Step 5: Resolve hostname to IP
    ips, err := net.LookupIP(hostname)
    if err != nil {
        return fmt.Errorf("DNS resolution failed: %w", err)
    }

    // Step 6: Validate IP addresses
    for _, ip := range ips {
        if err := v.validateIP(ip); err != nil {
            return fmt.Errorf("invalid IP for %s: %w", hostname, err)
        }
    }

    // Step 7: Validate port
    if parsed.Port() != "" && parsed.Port() != "443" {
        return fmt.Errorf("non-standard port not allowed: %s", parsed.Port())
    }

    // Step 8: Validate path (prevent path traversal)
    if strings.Contains(parsed.Path, "..") {
        return fmt.Errorf("path traversal detected")
    }

    return nil
}

// validateIP checks if IP is in private/reserved ranges
func (v *URLValidator) validateIP(ip net.IP) error {
    // Private IPv4 ranges
    privateRanges := []string{
        "10.0.0.0/8",
        "172.16.0.0/12",
        "192.168.0.0/16",
        "127.0.0.0/8",      // Loopback
        "169.254.0.0/16",   // Link-local
        "0.0.0.0/8",        // Current network
        "224.0.0.0/4",      // Multicast
        "240.0.0.0/4",      // Reserved
    }

    // AWS metadata IP
    if ip.String() == "169.254.169.254" {
        return fmt.Errorf("AWS metadata endpoint blocked")
    }

    for _, cidr := range privateRanges {
        _, ipnet, _ := net.ParseCIDR(cidr)
        if ipnet.Contains(ip) {
            return fmt.Errorf("private IP range blocked: %s", cidr)
        }
    }

    // Block IPv6 private ranges
    if ip.To4() == nil {
        if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
            return fmt.Errorf("private IPv6 address blocked")
        }
        // Block fc00::/7 (ULA)
        if ip[0] == 0xfc || ip[0] == 0xfd {
            return fmt.Errorf("IPv6 ULA blocked")
        }
    }

    return nil
}

// ValidateBeforeRequest performs DNS rebinding protection
func (v *URLValidator) ValidateBeforeRequest(rawURL string) error {
    // Re-validate URL before making request
    // This prevents DNS rebinding (TOCTOU) attacks
    return v.Validate(rawURL)
}
```

### 8.2 Action-Specific Validation

```go
// ValidateWeatherPayload validates weather action arguments
func ValidateWeatherPayload(payload *models.WebActionPayload) error {
    if payload.Action != "fetch_weather" {
        return fmt.Errorf("invalid action for weather handler")
    }

    // Validate URL matches weather.gov
    if !strings.HasPrefix(payload.URL, "https://api.weather.gov/gridpoints/") {
        return fmt.Errorf("invalid weather API URL")
    }

    // Validate days argument
    if days, ok := payload.Arguments["days"].(float64); ok {
        if days < 1 || days > 14 {
            return fmt.Errorf("days must be 1-14")
        }
    }

    return nil
}

// ValidateGolfPayload validates golf action arguments
func ValidateGolfPayload(payload *models.WebActionPayload) error {
    // Validate OAuth config present
    if payload.AuthConfig == nil || payload.AuthConfig.Type != "oauth_password" {
        return fmt.Errorf("OAuth authentication required for golf action")
    }

    // Validate golfer_id is numeric
    golferID, ok := payload.Arguments["golfer_id"].(string)
    if !ok {
        return fmt.Errorf("golfer_id required")
    }

    if !regexp.MustCompile(`^\d+$`).MatchString(golferID) {
        return fmt.Errorf("golfer_id must be numeric")
    }

    // Validate max_results
    if maxResults, ok := payload.Arguments["max_results"].(float64); ok {
        if maxResults < 1 || maxResults > 100 {
            return fmt.Errorf("max_results must be 1-100")
        }
    }

    return nil
}
```

---

## 9. OAuth 2.0 Security

### 9.1 OAuth Flow Security Analysis

**Current Design:** Password Grant Flow (Resource Owner Password Credentials)

**Security Issues:**
1. ⚠️ **Password grant is deprecated** in OAuth 2.1
2. ⚠️ Client credentials (`js1:v4secret`) hardcoded in design doc
3. ⚠️ Token refresh not implemented
4. ⚠️ Token revocation not implemented

### 9.2 Secure OAuth Implementation

```go
package auth

import (
    "context"
    "crypto/subtle"
    "encoding/json"
    "fmt"
    "net/http"
    "net/url"
    "strings"
    "sync"
    "time"
)

type OAuthClient struct {
    httpClient   *http.Client
    secretsCache *secrets.SecretCache
    tokenCache   *TokenCache
    logger       *slog.Logger
}

// TokenCache provides thread-safe OAuth token caching
type TokenCache struct {
    tokens map[string]*CachedToken
    mu     sync.RWMutex
}

type CachedToken struct {
    AccessToken  string
    RefreshToken string
    ExpiresAt    time.Time
    Scope        string
}

// GetToken retrieves or fetches OAuth token with security best practices
func (c *OAuthClient) GetToken(ctx context.Context, authConfig *models.AuthConfig) (string, error) {
    // Check cache first
    cacheKey := c.getCacheKey(authConfig)

    c.tokenCache.mu.RLock()
    cached, exists := c.tokenCache.tokens[cacheKey]
    c.tokenCache.mu.RUnlock()

    if exists && time.Now().Before(cached.ExpiresAt.Add(-5*time.Minute)) {
        c.logger.InfoContext(ctx, "using cached OAuth token",
            slog.String("cache_key_hash", hashCacheKey(cacheKey)),
            slog.Time("expires_at", cached.ExpiresAt),
        )
        return cached.AccessToken, nil
    }

    // Fetch new token
    token, err := c.fetchToken(ctx, authConfig)
    if err != nil {
        // Log security event (without credentials)
        c.logger.ErrorContext(ctx, "OAuth authentication failed",
            slog.String("error_type", "auth_failure"),
            slog.String("endpoint", authConfig.TokenURL),
        )
        return "", fmt.Errorf("OAuth authentication failed: %w", err)
    }

    // Cache token
    c.tokenCache.mu.Lock()
    c.tokenCache.tokens[cacheKey] = token
    c.tokenCache.mu.Unlock()

    // Log successful authentication (without token)
    c.logger.InfoContext(ctx, "OAuth authentication successful",
        slog.String("scope", token.Scope),
        slog.Time("expires_at", token.ExpiresAt),
    )

    return token.AccessToken, nil
}

// fetchToken performs OAuth password grant flow with security hardening
func (c *OAuthClient) fetchToken(ctx context.Context, authConfig *models.AuthConfig) (*CachedToken, error) {
    // Get credentials from Secrets Manager
    creds, err := c.secretsCache.GetGolfCredentials(ctx)
    if err != nil {
        return nil, fmt.Errorf("failed to retrieve credentials: %w", err)
    }

    // Build form data (NOT logged)
    formData := url.Values{
        "grant_type":    {"password"},
        "username":      {creds.Username},
        "password":      {creds.Password},
        "client_id":     {"js1"},
        "client_secret": {"v4secret"},
        "scope":         {authConfig.Scope},
    }

    // Create request
    req, err := http.NewRequestWithContext(ctx, "POST", authConfig.TokenURL, strings.NewReader(formData.Encode()))
    if err != nil {
        return nil, err
    }

    // Set headers (from authConfig, not hardcoded)
    req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
    req.Header.Set("Accept", "application/json")
    for k, v := range authConfig.Headers {
        req.Header.Set(k, v)
    }
    req.Header.Set("User-Agent", "rez-agent/1.0")

    // Execute request
    startTime := time.Now()
    resp, err := c.httpClient.Do(req)
    duration := time.Since(startTime)

    if err != nil {
        return nil, fmt.Errorf("HTTP request failed: %w", err)
    }
    defer resp.Body.Close()

    // Check status code
    if resp.StatusCode != http.StatusOK {
        body, _ := io.ReadAll(resp.Body)
        // DO NOT log response body (may contain error details with credentials)
        return nil, fmt.Errorf("OAuth endpoint returned status %d", resp.StatusCode)
    }

    // Parse response
    var tokenResp OAuthTokenResponse
    if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
        return nil, fmt.Errorf("failed to parse token response: %w", err)
    }

    // Validate token response
    if err := c.validateTokenResponse(&tokenResp); err != nil {
        return nil, fmt.Errorf("invalid token response: %w", err)
    }

    // Create cached token
    expiresAt := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
    cachedToken := &CachedToken{
        AccessToken:  tokenResp.AccessToken,
        RefreshToken: tokenResp.RefreshToken,
        ExpiresAt:    expiresAt,
        Scope:        tokenResp.Scope,
    }

    return cachedToken, nil
}

// validateTokenResponse validates OAuth token response
func (c *OAuthClient) validateTokenResponse(resp *OAuthTokenResponse) error {
    if resp.AccessToken == "" {
        return fmt.Errorf("missing access_token")
    }

    if resp.TokenType != "Bearer" {
        return fmt.Errorf("unexpected token_type: %s", resp.TokenType)
    }

    if resp.ExpiresIn <= 0 {
        return fmt.Errorf("invalid expires_in: %d", resp.ExpiresIn)
    }

    // Validate JWT structure (3 parts separated by dots)
    parts := strings.Split(resp.AccessToken, ".")
    if len(parts) != 3 {
        return fmt.Errorf("invalid JWT structure")
    }

    return nil
}

// InvalidateToken removes token from cache (on auth failure)
func (c *OAuthClient) InvalidateToken(authConfig *models.AuthConfig) {
    cacheKey := c.getCacheKey(authConfig)
    c.tokenCache.mu.Lock()
    delete(c.tokenCache.tokens, cacheKey)
    c.tokenCache.mu.Unlock()
}

// getCacheKey generates cache key (NOT logged, contains credentials)
func (c *OAuthClient) getCacheKey(authConfig *models.AuthConfig) string {
    return fmt.Sprintf("%s:%s:%s", authConfig.TokenURL, authConfig.SecretName, authConfig.Scope)
}

// hashCacheKey creates loggable hash of cache key
func hashCacheKey(key string) string {
    h := sha256.Sum256([]byte(key))
    return hex.EncodeToString(h[:8])  // First 8 bytes
}

type OAuthTokenResponse struct {
    AccessToken  string `json:"access_token"`
    RefreshToken string `json:"refresh_token"`
    ExpiresIn    int    `json:"expires_in"`
    TokenType    string `json:"token_type"`
    Scope        string `json:"scope"`
}
```

### 9.3 OAuth Security Checklist

- ✅ Use HTTPS for token endpoint
- ✅ Validate token response structure
- ✅ Cache tokens with expiration
- ✅ Invalidate tokens on auth failures
- ⚠️ Token refresh not implemented (RECOMMENDED)
- ❌ Token revocation not implemented (OPTIONAL)
- ❌ Client credentials not rotated (HIGH RISK)

---

## 10. Logging & Monitoring Security

### 10.1 Secure Logging Standards

**Sensitive Data That Must Never Be Logged:**
1. OAuth access tokens / refresh tokens
2. Passwords (user credentials, client secrets)
3. API keys
4. Session tokens
5. PII (names, emails, phone numbers)
6. Full HTTP request/response bodies containing secrets

**Security Event Logging Requirements:**

```go
type SecurityLogger struct {
    logger *slog.Logger
}

// LogAuthenticationAttempt logs OAuth attempts (success/failure)
func (sl *SecurityLogger) LogAuthenticationAttempt(ctx context.Context, success bool, endpoint string, err error) {
    level := slog.LevelInfo
    if !success {
        level = slog.LevelWarn
    }

    sl.logger.Log(ctx, level, "oauth_authentication_attempt",
        slog.String("event_type", "authentication"),
        slog.Bool("success", success),
        slog.String("endpoint", endpoint),
        slog.String("error_code", getErrorCode(err)),  // NOT full error message
        slog.String("correlation_id", getCorrelationID(ctx)),
    )
}

// LogSecretAccess logs Secrets Manager access
func (sl *SecurityLogger) LogSecretAccess(ctx context.Context, secretName string) {
    sl.logger.InfoContext(ctx, "secret_accessed",
        slog.String("event_type", "secret_access"),
        slog.String("secret_name", secretName),
        slog.String("lambda_function", os.Getenv("AWS_LAMBDA_FUNCTION_NAME")),
        slog.String("correlation_id", getCorrelationID(ctx)),
    )
}

// LogURLValidationFailure logs SSRF prevention events
func (sl *SecurityLogger) LogURLValidationFailure(ctx context.Context, url string, reason string) {
    sl.logger.WarnContext(ctx, "url_validation_failed",
        slog.String("event_type", "security_violation"),
        slog.String("url", redactSensitiveParams(url)),
        slog.String("reason", reason),
        slog.String("correlation_id", getCorrelationID(ctx)),
    )
}

// LogHTTPRequest logs outbound requests (with redaction)
func (sl *SecurityLogger) LogHTTPRequest(ctx context.Context, req *http.Request, statusCode int, duration time.Duration) {
    sl.logger.InfoContext(ctx, "http_request_executed",
        slog.String("method", req.Method),
        slog.String("host", req.URL.Host),
        slog.String("path", redactPathParams(req.URL.Path)),
        slog.Int("status_code", statusCode),
        slog.Duration("duration", duration),
        slog.String("correlation_id", getCorrelationID(ctx)),
    )
}

// redactSensitiveParams removes query parameters that may contain secrets
func redactSensitiveParams(rawURL string) string {
    parsed, _ := url.Parse(rawURL)
    if parsed == nil {
        return "[INVALID_URL]"
    }

    query := parsed.Query()
    sensitiveParams := []string{"token", "api_key", "password", "secret"}

    for _, param := range sensitiveParams {
        if query.Has(param) {
            query.Set(param, "[REDACTED]")
        }
    }

    parsed.RawQuery = query.Encode()
    return parsed.String()
}
```

### 10.2 CloudWatch Logs Security

**Current Design Issues:**
1. No log group encryption mentioned
2. Retention period not security-focused
3. No automated log analysis for secrets

**Recommended Configuration:**

```go
// Pulumi infrastructure - encrypted log group
webactionLogGroup, err := cloudwatch.NewLogGroup(ctx, "rez-agent-webaction-logs", &cloudwatch.LogGroupArgs{
    Name:            pulumi.String("/aws/lambda/rez-agent-webaction-prod"),
    RetentionInDays: pulumi.Int(30),  // Balance security and compliance
    KmsKeyId:        kmsKey.Arn,      // Encrypt logs at rest
    Tags:            commonTags,
})

// CloudWatch Logs Insights - Detect token leakage
query := `
fields @timestamp, @message
| filter @message like /Bearer\s+[A-Za-z0-9\-_]+\.[A-Za-z0-9\-_]+/
    or @message like /password.*[:=]\s*[^\s]+/
    or @message like /api[_-]?key.*[:=]\s*[^\s]+/
| stats count() as leaks by bin(1h)
`

// CloudWatch Alarm for secret leakage
_, err = cloudwatch.NewMetricAlarm(ctx, "secret-leakage-alarm", &cloudwatch.MetricAlarmArgs{
    Name:              pulumi.String("rez-agent-secret-leakage-prod"),
    ComparisonOperator: pulumi.String("GreaterThanThreshold"),
    EvaluationPeriods: pulumi.Int(1),
    Threshold:         pulumi.Float64(1),
    AlarmDescription:  pulumi.String("Alert on potential secret leakage in logs"),
    // Use Logs Insights query as metric
})
```

### 10.3 Security Metrics

**Custom CloudWatch Metrics:**

```go
func publishSecurityMetrics(ctx context.Context, cwClient *cloudwatch.Client, stage string) {
    metrics := []types.MetricDatum{
        {
            MetricName: aws.String("OAuthAuthenticationFailures"),
            Value:      aws.Float64(1),
            Unit:       types.StandardUnitCount,
            Timestamp:  aws.Time(time.Now()),
            Dimensions: []types.Dimension{
                {Name: aws.String("Stage"), Value: aws.String(stage)},
                {Name: aws.String("Action"), Value: aws.String("fetch_golf_reservations")},
            },
        },
        {
            MetricName: aws.String("URLValidationFailures"),
            Value:      aws.Float64(1),
            Unit:       types.StandardUnitCount,
            Dimensions: []types.Dimension{
                {Name: aws.String("Stage"), Value: aws.String(stage)},
                {Name: aws.String("Reason"), Value: aws.String("ssrf_attempt")},
            },
        },
        {
            MetricName: aws.String("SecretAccessCount"),
            Value:      aws.Float64(1),
            Unit:       types.StandardUnitCount,
            Dimensions: []types.Dimension{
                {Name: aws.String("SecretName"), Value: aws.String("golf-credentials")},
            },
        },
    }

    cwClient.PutMetricData(ctx, &cloudwatch.PutMetricDataInput{
        Namespace:  aws.String("RezAgent/Security"),
        MetricData: metrics,
    })
}
```

---

## 11. Security Testing Plan

### 11.1 Static Application Security Testing (SAST)

**Tools:**
1. **gosec** - Go security linter
2. **semgrep** - Pattern-based static analysis
3. **nancy** - Dependency vulnerability scanner

**Configuration:**

```yaml
# .gosec.yml
{
  "severity": "medium",
  "confidence": "medium",
  "exclude": [],
  "rules": {
    "G101": {  // Look for hardcoded credentials
      "enabled": true,
      "pattern": "(password|passwd|pwd|secret|token|api[_-]?key)"
    },
    "G104": {  // Audit errors not checked
      "enabled": true
    },
    "G401": {  // Detect weak crypto
      "enabled": true
    },
    "G402": {  // TLS MinVersion
      "enabled": true
    }
  }
}
```

**CI/CD Integration:**
```bash
# GitHub Actions workflow
- name: Run gosec Security Scanner
  run: |
    go install github.com/securego/gosec/v2/cmd/gosec@latest
    gosec -fmt json -out gosec-report.json -severity medium ./...

- name: Check for vulnerabilities
  run: |
    go list -json -m all | nancy sleuth
```

### 11.2 Dynamic Application Security Testing (DAST)

**Test Cases:**

#### SSRF Testing
```bash
# Test 1: AWS Metadata endpoint
curl -X POST https://api.example.com/api/messages \
  -H "Content-Type: application/json" \
  -d '{
    "message_type": "web_action",
    "payload": {
      "url": "http://169.254.169.254/latest/meta-data/",
      "action": "fetch_weather"
    }
  }'
# Expected: 400 Bad Request (URL validation failure)

# Test 2: Private IP range
curl -X POST https://api.example.com/api/messages \
  -H "Content-Type: application/json" \
  -d '{
    "message_type": "web_action",
    "payload": {
      "url": "http://192.168.1.1/admin",
      "action": "fetch_weather"
    }
  }'
# Expected: 400 Bad Request (Private IP blocked)

# Test 3: DNS rebinding
curl -X POST https://api.example.com/api/messages \
  -H "Content-Type: application/json" \
  -d '{
    "message_type": "web_action",
    "payload": {
      "url": "http://malicious-rebind.example.com/",
      "action": "fetch_weather"
    }
  }'
# Expected: 400 Bad Request (Host not in allowlist)
```

#### OAuth Security Testing
```bash
# Test 4: Token leakage detection
# Monitor CloudWatch Logs after triggering OAuth flow
aws logs tail /aws/lambda/rez-agent-webaction-prod --follow | grep -i "Bearer\|token"
# Expected: No tokens logged

# Test 5: Invalid credentials handling
# Temporarily modify secret with incorrect password
aws secretsmanager put-secret-value \
  --secret-id rez-agent/golf/credentials \
  --secret-string '{"username":"test","password":"wrong","golfer_id":"123"}'
# Trigger action, verify:
# - Proper error handling
# - No credential leakage in logs
# - Security event logged
```

#### IAM Permission Testing
```bash
# Test 6: Verify least privilege
# Use policy simulator
aws iam simulate-principal-policy \
  --policy-source-arn arn:aws:iam::ACCOUNT:role/rez-agent-webaction-role-prod \
  --action-names secretsmanager:PutSecretValue \
  --resource-arns arn:aws:secretsmanager:us-east-1:ACCOUNT:secret:rez-agent/golf/credentials
# Expected: "denied" (Lambda should NOT have PutSecretValue)
```

### 11.3 Penetration Testing Checklist

**Pre-Production Testing:**

| Test Category | Test Case | Expected Result | Priority |
|---------------|-----------|-----------------|----------|
| **SSRF** | AWS metadata endpoint | Blocked | P0 |
| **SSRF** | Private IP ranges (RFC1918) | Blocked | P0 |
| **SSRF** | Localhost (127.0.0.1) | Blocked | P0 |
| **SSRF** | DNS rebinding | Blocked | P0 |
| **OAuth** | Token in CloudWatch Logs | Not present | P0 |
| **OAuth** | Token in DynamoDB | Not stored | P0 |
| **OAuth** | Expired token handling | Refresh/re-auth | P1 |
| **OAuth** | Invalid credentials | Proper error, no leak | P1 |
| **IAM** | Access unauthorized secret | Denied | P0 |
| **IAM** | Modify IAM policy | Denied | P0 |
| **Input** | Negative days argument | Rejected | P2 |
| **Input** | XSS in golfer_id | Sanitized | P2 |
| **Encryption** | DynamoDB data at rest | Encrypted | P1 |
| **Encryption** | CloudWatch Logs | Encrypted | P2 |
| **Logging** | PII in logs | Redacted | P1 |
| **Logging** | Secrets in logs | Redacted | P0 |

### 11.4 Automated Security Scanning

**Infrastructure as Code Scanning:**
```bash
# Checkov - Pulumi/Terraform scanner
checkov -d infrastructure/ --framework pulumi
```

**Dependency Scanning:**
```bash
# govulncheck - Official Go vulnerability scanner
go install golang.org/x/vuln/cmd/govulncheck@latest
govulncheck ./...
```

**Container Scanning (if using Docker):**
```bash
# Trivy - Container vulnerability scanner
trivy image rez-agent-webaction:latest
```

---

## 12. Security Metrics & KPIs

### 12.1 Security Key Performance Indicators

| KPI | Target | Measurement | Alert Threshold |
|-----|--------|-------------|-----------------|
| **OAuth Authentication Failure Rate** | < 1% | CloudWatch Metric | > 5% in 5 minutes |
| **SSRF Prevention Rate** | 100% | URL validation failures / total requests | > 0 attempts in 1 hour |
| **Secret Access Anomalies** | 0 | Unexpected Secrets Manager calls | > 10 calls in 5 minutes |
| **Token Leakage Incidents** | 0 | CloudWatch Logs search for tokens | > 0 occurrences |
| **IAM Permission Violations** | 0 | CloudTrail access denied events | > 0 events |
| **Mean Time to Detect (MTTD) Security Events** | < 5 minutes | CloudWatch alarm latency | N/A |
| **Mean Time to Respond (MTTR) to Incidents** | < 30 minutes | Incident response time | N/A |
| **Secrets Rotation Compliance** | 100% | Rotated within 90 days | Overdue rotation |
| **Encryption Coverage** | 100% | DynamoDB tables encrypted | Any unencrypted table |
| **Audit Log Completeness** | 100% | Security events logged | Missing event types |

### 12.2 Security Dashboard (CloudWatch)

**Dashboard Widgets:**

1. **Authentication Failures (Time Series)**
   - Metric: `OAuthAuthenticationFailures`
   - Statistic: Sum
   - Period: 5 minutes

2. **SSRF Attempts (Bar Chart)**
   - Metric: `URLValidationFailures`
   - Dimensions: Reason (ssrf_attempt, private_ip, metadata_endpoint)
   - Period: 1 hour

3. **Secret Access Heatmap**
   - Metric: `SecretAccessCount`
   - Dimensions: SecretName, LambdaFunction
   - Period: 1 hour

4. **Top 10 Failed Actions (Table)**
   - Log Insights Query:
     ```
     fields @timestamp, action, error_code
     | filter event_type = "action_failed"
     | stats count() as failures by action
     | sort failures desc
     | limit 10
     ```

5. **Token Leakage Detection (Anomaly)**
   - Log Insights Query:
     ```
     fields @message
     | filter @message like /Bearer\s+[A-Za-z0-9]/
     | stats count() as potential_leaks
     ```

### 12.3 Security Alerting Rules

**CloudWatch Alarms:**

```go
// Critical: OAuth failures spike
_, err = cloudwatch.NewMetricAlarm(ctx, "oauth-failure-spike", &cloudwatch.MetricAlarmArgs{
    AlarmName:          pulumi.String("rez-agent-oauth-failure-spike-prod"),
    ComparisonOperator: pulumi.String("GreaterThanThreshold"),
    EvaluationPeriods:  pulumi.Int(1),
    MetricName:         pulumi.String("OAuthAuthenticationFailures"),
    Namespace:          pulumi.String("RezAgent/Security"),
    Period:             pulumi.Int(300),  // 5 minutes
    Statistic:          pulumi.String("Sum"),
    Threshold:          pulumi.Float64(5),
    AlarmDescription:   pulumi.String("CRITICAL: Multiple OAuth failures detected"),
    TreatMissingData:   pulumi.String("notBreaching"),
    AlarmActions:       pulumi.StringArray{snsTopicArn},  // PagerDuty
})

// Critical: SSRF attempt detected
_, err = cloudwatch.NewMetricAlarm(ctx, "ssrf-attempt", &cloudwatch.MetricAlarmArgs{
    AlarmName:          pulumi.String("rez-agent-ssrf-attempt-prod"),
    ComparisonOperator: pulumi.String("GreaterThanThreshold"),
    EvaluationPeriods:  pulumi.Int(1),
    MetricName:         pulumi.String("URLValidationFailures"),
    Namespace:          pulumi.String("RezAgent/Security"),
    Period:             pulumi.Int(60),
    Statistic:          pulumi.String("Sum"),
    Threshold:          pulumi.Float64(0),  // Alert on ANY attempt
    AlarmDescription:   pulumi.String("CRITICAL: SSRF attack attempt detected"),
    AlarmActions:       pulumi.StringArray{securityTeamSNS},
})
```

---

## 13. Implementation Security Checklist

### 13.1 Pre-Implementation (Design Phase)

- [x] Threat model completed (STRIDE)
- [x] Attack surface analysis performed
- [x] Risk assessment matrix created
- [x] IAM policies reviewed for least privilege
- [ ] **CRITICAL**: SSRF prevention design approved
- [ ] **CRITICAL**: OAuth token handling design approved
- [ ] **HIGH**: Secrets rotation strategy defined
- [ ] **HIGH**: PII encryption strategy approved
- [ ] Compliance requirements documented (GDPR, SOC 2)

### 13.2 Development Phase

**Critical Security Controls:**
- [ ] **V-001**: URL allowlist implemented and tested
- [ ] **V-001**: Private IP range validation implemented
- [ ] **V-001**: AWS metadata endpoint blocked
- [ ] **V-002**: OAuth token logging redaction enforced
- [ ] **V-002**: Static analysis for token leakage configured
- [ ] **V-003**: DynamoDB encryption at rest enabled
- [ ] **V-003**: KMS customer-managed key created
- [ ] **V-004**: Error message sanitization implemented
- [ ] **V-005**: IAM policies scoped to specific resources
- [ ] **V-006**: Secrets rotation Lambda created
- [ ] **V-007**: Security event logging implemented

**High Priority Controls:**
- [ ] Input validation for all action arguments
- [ ] HTTP client TLS 1.3 configuration
- [ ] Certificate pinning for Golf API
- [ ] Response size limits (10MB)
- [ ] Redirect following limits (2 max)
- [ ] OAuth token cache encryption
- [ ] CloudWatch Logs encryption enabled
- [ ] X-Ray tracing for security events

**Medium Priority Controls:**
- [ ] DNS rebinding protection
- [ ] HTTP timeout configuration
- [ ] Rate limiting on external APIs
- [ ] DynamoDB results field-level encryption
- [ ] Audit logging for all secret access
- [ ] Security metrics dashboard created

### 13.3 Testing Phase

**Security Testing:**
- [ ] SAST: gosec scan passed (0 high/critical findings)
- [ ] SAST: semgrep custom rules for secrets detection
- [ ] Dependency scanning: no known vulnerabilities
- [ ] SSRF testing: all test cases passed
- [ ] OAuth testing: no token leakage detected
- [ ] IAM testing: permission boundaries enforced
- [ ] Input validation testing: all edge cases covered
- [ ] Penetration testing: external assessment completed
- [ ] CloudWatch Logs: manual review for secrets

**Documentation:**
- [ ] Security architecture diagram updated
- [ ] Incident response runbook created
- [ ] Secrets rotation procedure documented
- [ ] Security monitoring dashboard created
- [ ] Compliance evidence collected

### 13.4 Pre-Production Deployment

**Infrastructure Security:**
- [ ] KMS keys created with rotation enabled
- [ ] Secrets Manager secrets created with encryption
- [ ] DynamoDB tables have encryption enabled
- [ ] CloudWatch Logs encrypted with KMS
- [ ] IAM roles follow least privilege
- [ ] S3 buckets (if any) have encryption and versioning
- [ ] VPC flow logs enabled (if Lambda in VPC)

**Operational Security:**
- [ ] CloudWatch alarms configured and tested
- [ ] SNS topics for security alerts created
- [ ] Security team notification workflow tested
- [ ] Backup and recovery procedures tested
- [ ] Incident response team identified
- [ ] On-call rotation for security incidents

### 13.5 Production Deployment

**Pre-Deployment Checklist:**
- [ ] Security review sign-off obtained
- [ ] Compliance team approval (if required)
- [ ] Penetration testing report reviewed
- [ ] All Critical (P0) vulnerabilities mitigated
- [ ] All High (P1) vulnerabilities mitigated or accepted
- [ ] Deployment rollback plan tested
- [ ] Monitoring dashboards validated

**Post-Deployment Verification:**
- [ ] CloudWatch alarms are active
- [ ] Security metrics are publishing
- [ ] No secrets in CloudWatch Logs (24-hour verification)
- [ ] IAM permissions validated via testing
- [ ] External API connections working with TLS 1.3
- [ ] OAuth flow successful without token leakage
- [ ] SSRF prevention validated in production

### 13.6 Ongoing Security Operations

**Monthly:**
- [ ] Review CloudWatch security metrics
- [ ] Analyze CloudWatch Logs for anomalies
- [ ] Review IAM access advisor for unused permissions
- [ ] Check Secrets Manager rotation status
- [ ] Update dependency versions (security patches)

**Quarterly:**
- [ ] Conduct security audit of logs
- [ ] Review and update threat model
- [ ] Penetration testing (external)
- [ ] Compliance assessment (GDPR, SOC 2)
- [ ] Security training for development team

**Annually:**
- [ ] Full security architecture review
- [ ] Rotate KMS keys
- [ ] Review and update incident response procedures
- [ ] Third-party security assessment

---

## 14. Mitigation Roadmap

### 14.1 Critical Mitigations (P0) - Must Complete Before Production

**Timeline:** 2 weeks

| ID | Vulnerability | Mitigation | Owner | ETA | Status |
|----|--------------|------------|-------|-----|--------|
| V-001 | SSRF via URL injection | Implement URL allowlist + IP validation | Backend Dev | Week 1 | Not Started |
| V-002 | OAuth token leakage in logs | Implement logging redaction + static analysis | Backend Dev | Week 1 | Not Started |

**Acceptance Criteria:**
- URL validation blocks all private IPs, localhost, metadata endpoints
- Unit tests cover all SSRF attack vectors
- Static analysis tool detects token logging patterns
- Manual log review confirms no tokens present
- Penetration testing validates SSRF prevention

### 14.2 High Priority Mitigations (P1) - Complete Within 4 Weeks

**Timeline:** 4 weeks

| ID | Vulnerability | Mitigation | Owner | ETA | Status |
|----|--------------|------------|-------|-----|--------|
| V-003 | PII exposure in DynamoDB | Enable encryption at rest with KMS | DevOps | Week 2 | Not Started |
| V-004 | Secrets in error messages | Implement error sanitization | Backend Dev | Week 2 | Not Started |
| V-005 | IAM over-permissions | Scope IAM policies to specific resources | DevOps | Week 2 | Not Started |
| V-006 | Missing secrets rotation | Implement rotation Lambda | Backend Dev | Week 3 | Not Started |
| V-007 | Insufficient audit logging | Implement security event logging | Backend Dev | Week 3 | Not Started |

**Acceptance Criteria:**
- DynamoDB encryption validated with KMS key
- Error messages do not contain credential substrings
- IAM policy simulator confirms least privilege
- Secrets rotation tested and scheduled
- Security events appear in CloudWatch Logs

### 14.3 Medium Priority Mitigations (P2) - Complete Within 6 Weeks

**Timeline:** 6 weeks

| ID | Mitigation | Owner | ETA |
|----|-----------|-------|-----|
| V-008 | Implement certificate pinning for Golf API | Backend Dev | Week 4 |
| V-009 | Add OAuth token cache encryption | Backend Dev | Week 4 |
| V-010 | Comprehensive input validation | Backend Dev | Week 5 |
| V-011 | Enable CloudWatch Logs encryption | DevOps | Week 5 |

### 14.4 Low Priority Mitigations (P3) - Complete Within 8 Weeks

**Timeline:** 8 weeks

| ID | Mitigation | Owner | ETA |
|----|-----------|-------|-----|
| V-012 | Reduce log retention to 7 days (dev) | DevOps | Week 6 |
| V-013 | Add X-Ray tracing for security events | Backend Dev | Week 7 |

### 14.5 Continuous Security Improvements

**Phase 2 Enhancements (Post-Production):**
1. **Secrets Rotation Automation**
   - Automatic rotation every 90 days
   - Zero-downtime rotation with AWSPENDING/AWSCURRENT

2. **Advanced Threat Detection**
   - AWS GuardDuty integration
   - Anomaly detection for API call patterns
   - Behavioral analysis for secret access

3. **Compliance Automation**
   - Automated GDPR compliance reports
   - SOC 2 evidence collection
   - Audit trail retention (7 years)

4. **Security Testing Automation**
   - Weekly DAST scans
   - Continuous dependency scanning
   - Automated penetration testing (monthly)

---

## 15. Security Review Sign-Off

### 15.1 Approval Requirements

**Security Team Approval:**
- [ ] All Critical (P0) vulnerabilities mitigated
- [ ] High (P1) vulnerabilities mitigated or risk accepted
- [ ] Security testing completed successfully
- [ ] Incident response procedures documented

**Compliance Team Approval:**
- [ ] GDPR compliance assessed
- [ ] SOC 2 controls mapped
- [ ] Data retention policies defined
- [ ] Audit logging sufficient

**Engineering Team Acknowledgment:**
- [ ] Security checklist reviewed
- [ ] Mitigation roadmap understood
- [ ] Ongoing security operations planned

### 15.2 Risk Acceptance

**Accepted Risks (if any):**

| Risk ID | Description | Justification | Mitigation Plan | Acceptance Date |
|---------|-------------|---------------|-----------------|-----------------|
| [None] | | | | |

**Risk Acceptance Signatures:**
- Security Lead: __________________________ Date: __________
- Engineering Manager: ____________________ Date: __________
- Product Owner: __________________________ Date: __________

---

## 16. Appendices

### Appendix A: OWASP Top 10 Mapping

| OWASP Risk | Relevance | Addressed |
|------------|-----------|-----------|
| A01:2021 – Broken Access Control | High (IAM over-permissions) | ⚠️ PARTIAL |
| A02:2021 – Cryptographic Failures | High (No DynamoDB encryption) | ❌ NO |
| A03:2021 – Injection | Critical (SSRF) | ❌ NO |
| A04:2021 – Insecure Design | Medium (OAuth password grant) | ⚠️ PARTIAL |
| A05:2021 – Security Misconfiguration | Medium (Secrets Manager wildcard) | ⚠️ PARTIAL |
| A06:2021 – Vulnerable Components | Low (Go dependencies up to date) | ✅ YES |
| A07:2021 – Identification & Auth Failures | High (No OAuth rotation) | ❌ NO |
| A08:2021 – Software and Data Integrity | Medium (No code signing) | ⚠️ PARTIAL |
| A09:2021 – Logging & Monitoring Failures | High (Insufficient audit logs) | ❌ NO |
| A10:2021 – SSRF | Critical (Primary concern) | ❌ NO |

### Appendix B: Compliance Frameworks

**GDPR Articles:**
- Article 5 (Principles): Storage limitation, data minimization
- Article 17 (Right to erasure): 3-day TTL may be acceptable
- Article 25 (Data protection by design): Encryption required
- Article 30 (Records of processing): Audit logging required
- Article 32 (Security of processing): Encryption, pseudonymization
- Article 33 (Breach notification): 72-hour notification required

**SOC 2 Trust Services Criteria:**
- CC6.1: Logical and physical access controls
- CC6.6: Encryption of sensitive data
- CC6.7: Transmission of data
- CC7.2: System monitoring
- CC7.3: Anomaly detection

### Appendix C: Security Tools & Resources

**Recommended Tools:**
1. **gosec** - https://github.com/securego/gosec
2. **semgrep** - https://semgrep.dev/
3. **nancy** - https://github.com/sonatype-nexus-community/nancy
4. **trivy** - https://github.com/aquasecurity/trivy
5. **checkov** - https://www.checkov.io/

**Security Resources:**
- OWASP ASVS: https://owasp.org/www-project-application-security-verification-standard/
- AWS Security Best Practices: https://aws.amazon.com/architecture/security-identity-compliance/
- CWE Top 25: https://cwe.mitre.org/top25/

---

## Document Revision History

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2025-10-23 | DevSecOps Security Specialist | Initial security audit report |

---

**End of Security Audit Report**

**Prepared by:** DevSecOps Security Specialist
**Review Status:** Complete
**Recommendation:** Conditional Approval - Implement Critical & High priority mitigations before production deployment.
