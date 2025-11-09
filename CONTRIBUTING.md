# Contributing to Go Rate Limiter

Thank you for your interest in contributing to the Go Rate Limiter project! This document provides guidelines and instructions for contributing.

## Code of Conduct

This project adheres to a code of conduct. By participating, you are expected to uphold this code. Please report unacceptable behavior to the project maintainers.

## How to Contribute

### Reporting Bugs

1. Check if the bug has already been reported in the Issues section
2. If not, create a new issue with:
   - Clear, descriptive title
   - Steps to reproduce
   - Expected vs actual behavior
   - Environment details (OS, Go version, etc.)
   - Any relevant logs or screenshots

### Suggesting Enhancements

1. Check if the enhancement has already been suggested
2. Create a new issue with:
   - Clear description of the enhancement
   - Use cases and benefits
   - Potential implementation approach

### Pull Requests

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Make your changes
4. Add tests for new functionality
5. Ensure all tests pass (`make test`)
6. Run linters (`make lint`)
7. Format your code (`make fmt`)
8. Commit your changes with clear messages
9. Push to your branch
10. Open a Pull Request

## Development Setup

```bash
# Clone your fork
git clone https://github.com/your-username/go-rate-limiter.git
cd go-rate-limiter

# Install dependencies
make install-deps

# Install development tools
make install-tools

# Run tests
make test

# Run benchmarks
make benchmark
```

## Coding Standards

### Go Style

- Follow the [official Go style guide](https://golang.org/doc/effective_go.html)
- Use `gofmt` for formatting
- Use `golangci-lint` for linting
- Write clear, self-documenting code
- Add comments for exported functions and complex logic

### Testing

- Write unit tests for new functionality
- Aim for >80% code coverage
- Include benchmark tests for performance-critical code
- Use table-driven tests where appropriate

### Commit Messages

Follow the conventional commits format:

```
type(scope): subject

body

footer
```

Types:
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation changes
- `test`: Test changes
- `refactor`: Code refactoring
- `perf`: Performance improvements
- `chore`: Build/tooling changes

Example:
```
feat(algorithms): add leaky bucket algorithm

Implemented the leaky bucket algorithm for rate limiting.
This provides constant outflow rate and better handles
burst traffic compared to token bucket.

Closes #123
```

## Testing Guidelines

### Unit Tests

- Test all public APIs
- Test edge cases and error conditions
- Use meaningful test names
- Keep tests independent and isolated

### Benchmark Tests

- Add benchmarks for performance-critical code
- Include memory allocation metrics
- Test with realistic workloads

### Integration Tests

- Test interaction with Redis
- Test HTTP API endpoints
- Use test containers where possible

## Performance Considerations

- Minimize allocations in hot paths
- Use `sync.Pool` for frequently allocated objects
- Profile code with `pprof`
- Document performance characteristics
- Add benchmarks for performance-critical changes

## Documentation

- Update README.md for user-facing changes
- Add godoc comments for all exported symbols
- Update API documentation for endpoint changes
- Add examples for new features

## Release Process

1. Update CHANGELOG.md
2. Update version numbers
3. Create release PR
4. Tag release after merge
5. Create GitHub release with notes

## Questions?

Feel free to open an issue for questions or join our community discussions.

Thank you for contributing!
