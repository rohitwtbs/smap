# Testing sMAP

This document describes how to run the test suites for sMAP.

## 1. Running with Docker (Recommended)

The easiest way to run the tests is using the provided Docker environment, which includes all necessary dependencies (Python 2.7, Twisted, NumPy, SciPy, etc.).

### Build the Test Image
```bash
docker build -t smap:ci .
```

### Run All Python Tests
```bash
docker run --rm smap:ci trial smap
```

### Run Specific Test Suites
*   **Core Logic**: `docker run --rm smap:ci trial smap.test`
*   **Data Operators**: `docker run --rm smap:ci trial smap.ops.test`
*   **Archiver Cache**: `docker run --rm smap:ci trial smap.archiver.test`

### Run Schema Validation
```bash
docker run --rm smap:ci bash -c "cd schema/test && python test.py"
```

## 2. Running Locally

To run tests locally, you need Python 2.7 and the dependencies listed in `python/requirements.txt` and `Dockerfile`.

### Setup
```bash
cd python
export PYTHONPATH=$PYTHONPATH:.
```

### Run Tests
```bash
trial smap
```

## 3. Continuous Integration

The project is configured with GitLab CI. Every push triggers the `.gitlab-ci.yml` pipeline, which:
1. Builds the Docker image.
2. Runs unit tests for core logic and operators.
3. Runs archiver cache tests.
4. Validates Avro schemas.

Refer to `.gitlab-ci.yml` for the detailed pipeline configuration.
