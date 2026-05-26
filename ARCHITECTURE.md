# sMAP Architecture Documentation

This document provides a comprehensive architectural overview of the sMAP (Simple Measurement and Actuation Profile) system. It details the core components, inter-component communication, overall data flow, and the modernized infrastructure.

## System Architecture Diagrams

### High-Level Overview
![sMAP High Level Architecture](python/doc/en/2.0/resources/highlevel.png)

*Figure 1: The sMAP architecture consisting of Sources, Archivers, and Applications.*

### Modernized Archiver Internals (Haskell + TimescaleDB)
The sMAP archiver has been modernized to replace the legacy Python/Twisted stack with a high-performance **Haskell** core and **TimescaleDB**.

### Data Organization
![sMAP Collections and Timeseries](python/doc/en/2.0/resources/ts_collections.png)

*Figure 3: sMAP organizes data as time series objects within collections.*

## System Flow (Mermaid)

```mermaid
graph TD
    subgraph "External World"
        Sensors[Sensors/Devices]
        Actuators[Actuators]
    end

    subgraph "sMAP Source (Python Driver)"
        DriverLogic[Driver Logic]
        Collection[Collection Hierarchy]
        Reporting[Reporting & Disk Buffer]
    end

    subgraph "sMAP Archiver (Haskell)"
        ServantAPI["Servant REST API"]
        Megaparsec["sMAPQL Parser (Megaparsec)"]
        DbPool["Postgres Connection Pool"]
    end

    subgraph "Storage Layer (TimescaleDB)"
        Postgres[(Postgres: Metadata)]
        Hypertable[(TimescaleDB: Hypertable)]
    end

    subgraph "Clients"
        QueryClient[Query Client]
        Subscriber[Real-time Subscriber]
    end

    %% Data Flow
    Sensors -->|Data| DriverLogic
    DriverLogic --> Collection
    Collection --> Reporting
    Reporting -->|HTTP POST| ServantAPI

    %% Ingestion Flow
    ServantAPI --> DbPool
    DbPool --> Postgres
    DbPool --> Hypertable

    %% Query Flow
    QueryClient -->|sMAPQL| ServantAPI
    ServantAPI --> Megaparsec
    Megaparsec --> DbPool
    DbPool --> Postgres
    DbPool --> Hypertable
    ServantAPI -->> QueryClient

    %% Actuation Flow
    QueryClient -.->|Actuation| DriverLogic
    DriverLogic -.-> Actuators
```

## 1. Core Components

The sMAP architecture consists of several main components that work together to organize and distribute time-series data:

*   **sMAP Source (Driver)**: The primary entry point for data. It is a Python application that interfaces with external systems to collect data. Internally, data is represented as `Timeseries` objects arranged within a `Collection` hierarchy (`python/smap/core.py`).
*   **Haskell Archiver**: The central service responsible for aggregating data. It has been rewritten in **Haskell** for superior concurrency and type safety.
    *   **Servant API**: Provides type-safe REST endpoints (`/add` and `/api/query`).
    *   **Megaparsec Parser**: A robust, monadic parser for the sMAP Query Language (sMAPQL).
*   **TimescaleDB (PostgreSQL)**: The primary storage backend.
    *   **Metadata**: Stored in the `stream` table using `hstore` for flexible key-value tagging.
    *   **Readings**: Stored in the `data` hypertable, optimized for time-series ingestion and retrieval.

## 2. Database Schema

The sMAP Archiver uses a unified PostgreSQL/TimescaleDB database to store both system state and time-series data.

### Core Tables

| Table | Purpose | Key Columns |
| :--- | :--- | :--- |
| `subscription` | Stores sMAP source registrations and API keys. | `id`, `uuid`, `key` |
| `stream` | Maps unique sMAP UUIDs to internal IDs and stores metadata. | `id`, `uuid`, `metadata` (HSTORE) |
| `data` | **TimescaleDB Hypertable.** Stores time-series readings. | `sid`, `time` (ms), `value` |

## 3. Overall Data Flow

1.  **Collection**: A Python driver acquires a measurement and adds it to the local `Timeseries` state.
2.  **Buffering**: The `Reporting` service buffers the data locally on disk to ensure reliability.
3.  **Transmission**: Periodically, the data is POSTed to the Haskell Archiver's `/add/[apikey]` endpoint.
4.  **Ingestion**: The Haskell Archiver validates the request via its type-safe API, flattens any nested metadata, and persists the results to TimescaleDB using a high-performance connection pool.
5.  **Querying**: Clients send sMAPQL queries to `/api/query`. The Haskell archiver parses the query using **Megaparsec**, identifies matching streams via `hstore` metadata filters, and retrieves the corresponding data points from the TimescaleDB hypertable.

## 4. Modernization Benefits

*   **Type Safety**: Haskell's strong type system eliminates runtime metadata errors.
*   **Concurrency**: Haskell's threaded runtime handles thousands of concurrent data streams more efficiently than the legacy Python/Twisted stack.
*   **Performance**: TimescaleDB provides native time-series optimizations (hypertables) that significantly outperform the legacy custom C++ `readingdb`.
*   **Containerization**: The entire stack is orchestrated via Docker, ensuring reproducible deployments.
