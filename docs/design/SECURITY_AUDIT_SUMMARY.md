# Web Action Processor - Security Audit Executive Summary

**Date:** 2025-10-23
**Status:** CONDITIONAL APPROVAL
**Risk Level:** MEDIUM (Requires Mitigation)

---

## Quick Assessment

### Overall Security Posture

🔴 **2 CRITICAL Issues** - Must fix before production
🟠 **5 HIGH Risk Issues** - Fix within 4 weeks
🟡 **8 MEDIUM Risk Issues** - Fix within 6 weeks
🟢 **6 LOW Risk Issues** - Address in future iterations

**Recommendation:** Implementation may proceed with **mandatory mitigation** of all Critical and High priority vulnerabilities before production deployment.

---

## Top 5 Security Risks

### 1. 🔴 CRITICAL: Server-Side Request Forgery (SSRF)
**Risk Score:** 8/10
**CWE-918**

**Problem:**
The design accepts URLs from DynamoDB without validation, allowing potential SSRF attacks to:
- AWS metadata service (169.254.169.254) → steal IAM credentials
- Internal VPC resources → network scanning
- Private IP ranges → access internal services

**Attack Scenario:**
```json
{
  "url": "http://169.254.169.254/latest/meta-data/iam/security-credentials/role-name",
  "action": "fetch_weather"
}
```

**Impact:** AWS account takeover, data exfiltration

**Mitigation Required:**
```go
// Implement strict URL allowlist
var allowedHosts = map[string]bool{
    "api.weather.gov":     true,
    "birdsfoot.cps.golf":  true,
}

// Validate before EVERY HTTP request
func validateURL(rawURL string) error {
    // 1. Parse URL
    // 2. Check allowlist
    // 3. Block private IPs (10.x, 192.168.x, 127.x, 169.254.x)
    // 4. Block AWS metadata
    // 5. Require HTTPS only
    // 6. Validate resolved IP != private range
}
```

---

### 2. 🔴 CRITICAL: OAuth Token Leakage in CloudWatch Logs
**Risk Score:** 9/10
**CWE-532**

**Problem:**
No enforcement mechanism to prevent developers from accidentally logging:
- OAuth access tokens (60-minute validity)
- Passwords from Secrets Manager
- Authorization headers

**Attack Scenario:**
```go
// Developer debugging code (accidentally deployed)
logger.InfoContext(ctx, "OAuth response", slog.String("token", accessToken))
```

**Impact:** Unauthorized Golf API access for 60 minutes, PII exposure

**Mitigation Required:**
1. **Implement logging redaction wrapper:**
```go
func (l *RedactedLogger) LogHTTPRequest(req *http.Request) {
    headers := make(map[string]string)
    for k, v := range req.Header {
        if k == "Authorization" {
            headers[k] = "[REDACTED]"
        } else {
            headers[k] = strings.Join(v, ",")
        }
    }
    l.logger.Info("HTTP request", slog.Any("headers", headers))
}
```

2. **Add static analysis linter:**
```bash
# Detect potential token logging
grep -r "slog.*token\|slog.*password\|logger.*Bearer" .
```

3. **CloudWatch Logs monitoring:**
```
# Detect leaked tokens
filter @message like /Bearer\s+[A-Za-z0-9\-_]+\./
```

---

### 3. 🟠 HIGH: PII Exposure in DynamoDB Results
**Risk Score:** 9/10
**GDPR Violation**

**Problem:**
Golf reservation API responses contain:
- Full names
- Email addresses
- Behavioral data (tee times)

Stored in DynamoDB **without encryption at rest** for 3 days.

**Compliance Impact:**
- GDPR Article 32: Requires encryption of personal data
- SOC 2 CC6.6: Requires encryption of sensitive data

**Mitigation Required:**
```go
// Pulumi infrastructure
resultsTable, err := dynamodb.NewTable(ctx, "web-action-results", &dynamodb.TableArgs{
    ServerSideEncryption: &dynamodb.TableServerSideEncryptionArgs{
        Enabled:   pulumi.Bool(true),
        KmsKeyArn: kmsKey.Arn,  // Customer-managed KMS key
    },
})
```

**Additionally:**
- Reduce TTL from 3 days → 24 hours
- Pseudonymize user identifiers
- Implement data deletion API for GDPR compliance

---

### 4. 🟠 HIGH: IAM Role Over-Permissions
**Risk Score:** 6/10
**CWE-269**

**Problem:**
Proposed IAM policy uses wildcards:
```json
{
  "Resource": "arn:aws:secretsmanager:*:*:secret:rez-agent/*"
}
```

Allows access to ANY secret with `rez-agent/` prefix, violating principle of least privilege.

**Mitigation Required:**
```json
{
  "Effect": "Allow",
  "Action": "secretsmanager:GetSecretValue",
  "Resource": "arn:aws:secretsmanager:us-east-1:ACCOUNT_ID:secret:rez-agent/golf/credentials-??????",
  "Condition": {
    "StringEquals": {"aws:RequestedRegion": "us-east-1"}
  }
}
```

Remove unnecessary permissions:
- `dynamodb:Query` → only needs `GetItem`
- `dynamodb:UpdateItem` on results table (not used)

---

### 5. 🟠 HIGH: Missing Secrets Rotation
**Risk Score:** 6/10
**PCI-DSS 8.2.4 Violation**

**Problem:**
Design states "Rotation: Manual (initially)" with no automation.

**Risks:**
- Credentials valid indefinitely
- No detection of compromised credentials
- Compliance violations (PCI-DSS requires 90-day rotation)

**Mitigation Required:**
Implement AWS Secrets Manager automatic rotation:
```go
golfSecret, err := secretsmanager.NewSecret(ctx, "golf-credentials", &secretsmanager.SecretArgs{
    RotationRules: &secretsmanager.SecretRotationRulesArgs{
        AutomaticallyAfterDays: pulumi.Int(90),
    },
})

// Create rotation Lambda
rotationLambda := lambda.NewFunction(ctx, "secrets-rotation", ...)
```

---

## Security Metrics Summary

| Metric | Current | Target | Status |
|--------|---------|--------|--------|
| SSRF Prevention | 0% | 100% | ❌ Not Implemented |
| Token Leakage Protection | 0% | 100% | ❌ Not Implemented |
| DynamoDB Encryption | 0% | 100% | ❌ Not Configured |
| IAM Least Privilege | 60% | 100% | ⚠️ Over-Permissive |
| Secrets Rotation | 0% | 100% | ❌ Not Implemented |
| Audit Logging Coverage | 40% | 100% | ⚠️ Incomplete |
| GDPR Compliance | 50% | 100% | ⚠️ Gaps Exist |

---

## Implementation Blockers

### MUST Complete Before Production (P0 - 2 Weeks)

1. **SSRF Prevention Implementation**
   - [ ] URL allowlist with `api.weather.gov` and `birdsfoot.cps.golf`
   - [ ] IP address validation (block private IPs, metadata endpoint)
   - [ ] DNS rebinding protection
   - [ ] Unit tests for all SSRF attack vectors
   - [ ] Penetration testing validation

2. **OAuth Token Logging Prevention**
   - [ ] Logging redaction wrapper for all HTTP requests
   - [ ] Static analysis linter for token logging patterns
   - [ ] CloudWatch Logs monitoring for leaked tokens
   - [ ] Code review checklist updated
   - [ ] Manual log inspection (24-hour post-deploy)

**Gate:** Production deployment BLOCKED until P0 items complete.

---

## High Priority Security Tasks (P1 - 4 Weeks)

3. **DynamoDB Encryption at Rest**
   - [ ] Create customer-managed KMS key
   - [ ] Enable encryption on results table
   - [ ] Validate encryption with AWS CLI

4. **Error Message Sanitization**
   - [ ] Implement error wrapping that redacts secrets
   - [ ] Test with intentionally wrong credentials

5. **IAM Policy Hardening**
   - [ ] Scope Secrets Manager permission to specific secret
   - [ ] Remove unnecessary DynamoDB permissions
   - [ ] Add region/account restrictions

6. **Secrets Rotation Automation**
   - [ ] Implement rotation Lambda
   - [ ] Test rotation without downtime
   - [ ] Schedule 90-day automatic rotation

7. **Security Event Logging**
   - [ ] Log OAuth authentication attempts (success/failure)
   - [ ] Log secret access events
   - [ ] Log URL validation failures
   - [ ] Create CloudWatch dashboard for security events

---

## Recommended Architecture Changes

### 1. Add URL Validation Layer

```
Current Flow:
EventBridge → Scheduler → DynamoDB → SNS → SQS → Lambda → HTTP Request

Secure Flow:
EventBridge → Scheduler → DynamoDB → SNS → SQS → Lambda
    → URL Validation Layer (allowlist + IP check)
    → HTTP Request
```

### 2. Implement Logging Abstraction

```go
// Create secure logger wrapper
type SecureLogger struct {
    logger *slog.Logger
}

func (sl *SecureLogger) LogHTTPRequest(req *http.Request) {
    // Automatically redact Authorization header
}

func (sl *SecureLogger) LogOAuthAttempt(success bool, endpoint string) {
    // Never log tokens, only success/failure
}
```

### 3. Add Security Monitoring

```
CloudWatch Logs → Logs Insights
    → Metric Filters
    → CloudWatch Alarms
    → SNS → Security Team Notification
```

---

## Compliance Status

### GDPR Compliance

| Article | Requirement | Status | Gap |
|---------|-------------|--------|-----|
| Article 5(e) | Storage limitation | ⚠️ PARTIAL | 3-day TTL may be excessive |
| Article 25 | Data protection by design | ❌ MISSING | No encryption |
| Article 32 | Security of processing | ❌ MISSING | No encryption at rest |
| Article 17 | Right to erasure | ❌ MISSING | No deletion API |

**Action Required:** Implement encryption, reduce TTL, add deletion API.

### SOC 2 Type II

| Control | Status | Evidence Needed |
|---------|--------|-----------------|
| CC6.1 - Logical Access | ⚠️ PARTIAL | IAM policies scoped to least privilege |
| CC6.6 - Encryption | ❌ MISSING | DynamoDB encryption enabled |
| CC7.2 - Monitoring | ⚠️ PARTIAL | Security event logging complete |
| CC7.3 - Incident Response | ⚠️ PARTIAL | Automated alerting configured |

---

## Testing Requirements

### Pre-Production Security Testing Checklist

**Static Analysis (SAST):**
- [ ] `gosec` scan: 0 high/critical findings
- [ ] `semgrep` custom rules for secrets detection
- [ ] Dependency scan: no known vulnerabilities

**Dynamic Testing (DAST):**
- [ ] SSRF testing: AWS metadata endpoint blocked
- [ ] SSRF testing: Private IP ranges blocked (10.x, 192.168.x, 127.x)
- [ ] OAuth testing: No tokens in CloudWatch Logs
- [ ] IAM testing: Cannot access unauthorized secrets
- [ ] Input validation: Negative/excessive values rejected

**Penetration Testing:**
- [ ] External penetration test completed
- [ ] SSRF attacks validated as blocked
- [ ] Token leakage not detected
- [ ] All findings remediated

---

## Estimated Effort

| Phase | Tasks | Effort | Timeline |
|-------|-------|--------|----------|
| **P0 - Critical** | SSRF prevention, token logging prevention | 40 hours | Week 1-2 |
| **P1 - High** | Encryption, IAM, secrets rotation, logging | 60 hours | Week 3-4 |
| **P2 - Medium** | Certificate pinning, input validation | 30 hours | Week 5-6 |
| **Testing** | Security testing, penetration testing | 20 hours | Week 7 |
| **Documentation** | Runbooks, incident response | 10 hours | Week 8 |
| **Total** | | **160 hours** | **8 weeks** |

---

## Approval Process

### Security Sign-Off Requirements

**Stage 1: Design Approval (Current Stage)**
- [x] Threat model completed
- [x] Security audit report reviewed
- [ ] Mitigation plan approved
- [ ] Engineering team acknowledges security requirements

**Stage 2: Implementation Approval**
- [ ] All P0 (Critical) vulnerabilities mitigated
- [ ] All P1 (High) vulnerabilities mitigated
- [ ] Security testing completed
- [ ] Code review with security focus

**Stage 3: Production Deployment Approval**
- [ ] Penetration testing passed
- [ ] Security monitoring configured
- [ ] Incident response procedures documented
- [ ] Compliance requirements met (GDPR, SOC 2)

---

## Next Steps

### Immediate Actions (This Week)

1. **Engineering Team:**
   - Review full security audit report (`WEB_ACTION_SECURITY_AUDIT.md`)
   - Prioritize P0 mitigations in sprint planning
   - Assign owners for each mitigation task

2. **Security Team:**
   - Approve mitigation roadmap
   - Schedule security testing sessions
   - Prepare penetration testing scope

3. **DevOps Team:**
   - Create KMS keys for encryption
   - Update Pulumi infrastructure for DynamoDB encryption
   - Review IAM policies

### Week 1-2: Critical Mitigations

- Implement SSRF prevention (URL validation)
- Implement logging redaction
- Write unit tests for security controls
- Begin static analysis integration

### Week 3-4: High Priority Mitigations

- Enable DynamoDB encryption
- Implement secrets rotation
- Harden IAM policies
- Add security event logging

### Week 5-8: Complete & Test

- Medium priority mitigations
- Security testing (SAST, DAST, penetration testing)
- Documentation
- Production deployment preparation

---

## Key Contacts

**Security Questions:**
- Security Team Lead: [Contact Info]
- DevSecOps Engineer: [Contact Info]

**Implementation Questions:**
- Backend Lead: [Contact Info]
- DevOps Lead: [Contact Info]

**Compliance Questions:**
- Compliance Officer: [Contact Info]

---

## Resources

**Full Reports:**
- Comprehensive Security Audit: `/workspaces/rez_agent/docs/design/WEB_ACTION_SECURITY_AUDIT.md`
- Technical Design: `/workspaces/rez_agent/docs/design/web-action-processor-design.md`

**Security Tools:**
- gosec: https://github.com/securego/gosec
- semgrep: https://semgrep.dev/
- AWS IAM Policy Simulator: https://policysim.aws.amazon.com/

**Compliance Frameworks:**
- OWASP ASVS: https://owasp.org/www-project-application-security-verification-standard/
- AWS Security Best Practices: https://aws.amazon.com/architecture/security-identity-compliance/
- GDPR Developer Guide: https://gdpr.eu/

---

**Document Status:** ✅ Complete
**Next Review:** After P0 mitigations implemented
**Last Updated:** 2025-10-23
