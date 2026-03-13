# SMAP
https://softwaredefinedbuildings.github.io/smap/

## Getting Started

### Installation

#### Python

To setup a clean environment, create a new python virtual environment with:

    virtualenv venv

Before proceeding with installation, make sure you've sourced the virtual environment with `source venv/bin/activate`.

You must install the dependencies listed in `requirements.txt` before installing smap. Do this by issuing the following:

    pip install -r osx_requirements.txt

After the dependencies are installed, run the installation:

    python setup.py install

## Documentation

Full documentation is located under [`python/doc/en/2.0/`](python/doc/en/2.0/).

### Overview
- [Introduction](python/doc/en/2.0/intro.rst) — Architecture overview: sources, archiver, and frontends
- [Installation](python/doc/en/2.0/install.rst) — Installing the sMAP library and dependencies

### Tutorials
- [Getting Started Tutorial](python/doc/en/2.0/tutorial.rst) — Start a driver, write a custom driver, send data to the archiver
- [Archiver Tutorial](python/doc/en/2.0/archiver_tutorial.rst) — Configuring queries and downloading data

### Core Library
- [Core API Reference](python/doc/en/2.0/core.rst) — `SmapInstance`, `Timeseries`, `Collection`, and `Reporting` classes
- [Internals & Configuration](python/doc/en/2.0/internals.rst) — Config file syntax, server settings, SSL, programmatic usage

### Drivers
- [Driver Patterns](python/doc/en/2.0/drivers.rst) — Periodic scraping, XML/XSLT transforms, and actuation
- [Driver Index](python/doc/en/2.0/driver_index.rst) — Catalog of all bundled drivers (meters, weather, ISO markets, etc.)

### Archiver
- [Archiver Install](python/doc/en/2.0/archiver_install.rst) — Setting up the archiver stack
- [Archiver Manual](python/doc/en/2.0/archiver_manual.rst) — Step-by-step: readingdb, PostgreSQL, powerdb2, archiver service
- [Archiver Query Language](python/doc/en/2.0/archiver.rst) — REST API and SQL-like query DSL

### Data Access
- [Python Client](python/doc/en/2.0/python_access.rst) — `SmapClient` for querying and plotting with numpy/matplotlib
- [R Client](python/doc/en/2.0/R_access.rst) — `RSmap` package for querying and plotting in R

### Reference
- [CLI Tools](python/doc/en/2.0/tools.rst) — `smap-query`, `smap-tool`, `smap-monitize`, `smap-load`, `smap-load-csv`
- [Metadata Tags](python/doc/en/2.0/tags.rst) — Standard tag names for instrument, location, and custom metadata
- [Additional Resources](python/doc/en/2.0/additional.rst) — Papers, talks, and mailing list

## Build status 
[![Build Status](https://travis-ci.org/SoftwareDefinedBuildings/smap.svg?branch=master)](https://travis-ci.org/SoftwareDefinedBuildings/smap)
