# Security Policy

## Supported Versions

We actively maintain and provide security updates for the following versions of Scanorama:

| Version | Supported          |
| ------- | ------------------ |
| 0.7.x   | :white_check_mark: |
| 0.6.x   | :white_check_mark: |
| < 0.6   | :x:                |

## Reporting a Vulnerability

We take the security of Scanorama seriously. If you discover a security vulnerability, please report it to us responsibly.

### How to Report

1. **Do NOT** open a public GitHub issue for security vulnerabilities
2. **Email us directly** at: [security@scanorama.io](mailto:security@scanorama.io)
3. **Include the following information:**
   - Description of the vulnerability
   - Steps to reproduce the issue
   - Potential impact
   - Any suggested fixes or mitigations
   - Your contact information for follow-up

### Response Timeline

- **Acknowledgment**: We will acknowledge receipt of your report within 48 hours
- **Initial Assessment**: We will provide an initial assessment within 5 business days
- **Updates**: We will keep you informed of our progress throughout the investigation
- **Resolution**: We aim to resolve critical vulnerabilities within 30 days

### Responsible Disclosure

We kindly ask that you:
- Give us reasonable time to investigate and fix the issue
- Do not publicly disclose the vulnerability until we have released a fix
- Do not exploit the vulnerability beyond what is necessary to demonstrate it
- Act in good faith and avoid privacy violations or data destruction

## Security Measures

### Application Security

- **Input Validation**: All user inputs are validated and sanitized
- **SQL Injection Protection**: Parameterized queries and ORM usage
- **Authentication**: Secure API authentication mechanisms
- **Authorization**: Role-based access control for sensitive operations
- **Rate Limiting**: Protection against abuse and DoS attacks
- **HTTPS**: All API communications use TLS encryption

### Infrastructure Security

- **Container Security**: Regular base image updates and vulnerability scanning
- **Database Security**: Encrypted connections and secure credential management
- **CI/CD Security**: Automated security scanning in our build pipeline
- **Dependency Management**: Regular dependency updates and vulnerability scanning

### Network Scanning Security

- **Ethical Usage**: Scanorama is designed for authorized network assessment only
- **Rate Limiting**: Built-in protections to avoid overwhelming target systems
- **Logging**: Comprehensive audit trails for all scanning activities
- **Access Controls**: Administrative features require proper authentication

## Security Best Practices for Users

### Deployment Security

1. **Use HTTPS**: Always deploy Scanorama behind HTTPS
2. **Secure Database**: Use strong passwords and encrypted connections
3. **Network Isolation**: Deploy in a secure network environment
4. **Regular Updates**: Keep Scanorama and its dependencies up to date
5. **Monitor Logs**: Review security logs regularly

### Configuration Security

1. **Strong Passwords**: Use strong, unique passwords for all accounts
2. **Environment Variables**: Store sensitive configuration in environment variables
3. **File Permissions**: Secure configuration files with appropriate permissions
4. **Database Credentials**: Rotate database credentials regularly
5. **API Keys**: Protect API keys and rotate them periodically

### Operational Security

1. **Authorized Scanning Only**: Only scan networks you own or have explicit permission to test
2. **Legal Compliance**: Ensure compliance with local laws and regulations
3. **Documentation**: Maintain records of authorized scanning activities
4. **Incident Response**: Have a plan for handling security incidents

## Known Security Considerations

### Network Scanning Risks

- Scanorama performs active network scanning which may be detected by security systems
- Excessive scanning may impact network performance
- Some security tools may flag scanning activity as suspicious
- Always obtain proper authorization before scanning networks

### API Security

- The API provides access to sensitive network information
- Ensure proper authentication and authorization controls
- Monitor API usage for unusual patterns
- Implement appropriate rate limiting

## Security Updates

Security updates are distributed through:
- GitHub Releases with security advisories
- Package manager updates (npm, Go modules)
- Docker image updates
- Direct email notifications for critical vulnerabilities

## Compliance

Scanorama is designed to support:
- SOC 2 compliance requirements
- ISO 27001 security standards
- GDPR data protection requirements (where applicable)
- Industry security best practices

## Security Resources

- [OWASP Top 10](https://owasp.org/www-project-top-ten/)
- [NIST Cybersecurity Framework](https://www.nist.gov/cyberframework)
- [CIS Controls](https://www.cisecurity.org/controls/)
- [SANS Security Guidelines](https://www.sans.org/security-resources/)

## Contact Information

For security-related questions or concerns:
- **Email**: [security@scanorama.io](mailto:security@scanorama.io)
- **Maintainer**: [@anstrom](https://github.com/anstrom)
- **Project**: [Scanorama on GitHub](https://github.com/anstrom/scanorama)

---

**Note**: This security policy is subject to change. Please check back regularly for updates.

Last updated: August 15, 2025