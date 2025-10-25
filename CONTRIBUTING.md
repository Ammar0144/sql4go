# Contributing to sql4go

First off, thank you for considering contributing to sql4go! It's people like you that make sql4go such a great tool.

## Code of Conduct

This project and everyone participating in it is governed by our commitment to providing a welcoming and inspiring community for all.

## How Can I Contribute?

### Reporting Bugs

Before creating bug reports, please check the issue list as you might find out that you don't need to create one. When you are creating a bug report, please include as many details as possible:

* **Use a clear and descriptive title** for the issue to identify the problem.
* **Describe the exact steps which reproduce the problem** in as many details as possible.
* **Provide specific examples to demonstrate the steps**.
* **Describe the behavior you observed after following the steps** and point out what exactly is the problem with that behavior.
* **Explain which behavior you expected to see instead and why.**
* **Include Go version, GORM version, and sql4go version**.

### Suggesting Enhancements

Enhancement suggestions are tracked as GitHub issues. When creating an enhancement suggestion, please include:

* **Use a clear and descriptive title** for the issue to identify the suggestion.
* **Provide a step-by-step description of the suggested enhancement** in as many details as possible.
* **Provide specific examples to demonstrate the steps** or provide code snippets.
* **Explain why this enhancement would be useful** to most sql4go users.

### Pull Requests

* Fill in the required template
* Do not include issue numbers in the PR title
* Follow the Go coding style
* Include thoughtfully-worded, well-structured tests
* Document new code
* End all files with a newline

## Development Process

1. Fork the repo
2. Create a new branch from `main`:
   ```bash
   git checkout -b feature/my-new-feature
   ```
3. Make your changes
4. Run tests (when available):
   ```bash
   go test ./...
   ```
5. Run formatting:
   ```bash
   go fmt ./...
   ```
6. Commit your changes:
   ```bash
   git commit -am 'Add some feature'
   ```
7. Push to the branch:
   ```bash
   git push origin feature/my-new-feature
   ```
8. Create a new Pull Request

## Style Guidelines

### Go Style Guide

* Follow the official [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
* Run `go fmt` before committing
* Run `go vet` to catch common mistakes
* Use meaningful variable and function names
* Add comments for exported functions and types
* Keep functions small and focused

### Git Commit Messages

* Use the present tense ("Add feature" not "Added feature")
* Use the imperative mood ("Move cursor to..." not "Moves cursor to...")
* Limit the first line to 72 characters or less
* Reference issues and pull requests liberally after the first line

## Project Structure

```
sql4go/
├── gensql4go.go       # Main package exports
├── pkg/
│   ├── db/            # Database manager and configuration
│   ├── redis/         # Redis manager and caching
│   └── repository/    # Generic repository implementation
```

## Questions?

Feel free to open an issue with your question or reach out to the maintainers.

Thank you for contributing! 🎉
