# Contributing to MediaTX

Thank you for your interest in contributing to MediaTX! 🎉

## How to Contribute

### Reporting Bugs

1. Check if the bug has already been reported in [Issues](https://github.com/media-tx/mediatx/issues)
2. If not, create a new issue with:
   - Clear title and description
   - Steps to reproduce
   - Expected vs actual behavior
   - System information (OS, Go version)

### Suggesting Features

1. Open an issue with the `enhancement` label
2. Describe the feature and its use case
3. Discuss with maintainers before implementation

### Pull Requests

1. Fork the repository
2. Create a feature branch: `git checkout -b feature/amazing-feature`
3. Make your changes
4. Run tests: `make test`
5. Format code: `make fmt`
6. Commit: `git commit -m 'Add amazing feature'`
7. Push: `git push origin feature/amazing-feature`
8. Open a Pull Request

## Development Setup

```bash
# Clone
git clone https://github.com/media-tx/mediatx.git
cd mediatx

# Build
make build

# Run
./mediatx

# Test
make test
```

## Code Style

- Follow standard Go conventions
- Use `gofmt` for formatting
- Write tests for new features
- Keep functions small and focused

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
