import psycopg2
import psycopg2.extras
import numpy as np
import logging

# This is a SQL-based shim for readingdb that stores data in the same Postgres
# instance as the metadata. It allows sMAP to run without the custom readingdb C++ server.

def db_setup(host, port):
    """No-op for the SQL shim."""
    pass

def _get_conn():
    from smap.archiver import settings
    db_conf = settings.conf['database']
    return psycopg2.connect(host=db_conf['host'],
                            database=db_conf['db'],
                            user=db_conf['user'],
                            password=db_conf['password'],
                            port=db_conf['port'])

def db_open(host=None, port=None):
    """Return a standard psycopg2 connection."""
    return _get_conn()

def db_close(conn):
    """Close the connection."""
    conn.close()

def db_add(conn, sid, data):
    """Insert readings into the 'data' table."""
    try:
        with conn.cursor() as cur:
            # data is a list of (timestamp_ms, flags, value)
            psycopg2.extras.execute_values(cur,
                "INSERT INTO data (sid, time, value) VALUES %s",
                [(sid, x[0], x[2]) for x in data])
            conn.commit()
    except Exception as e:
        logging.error("SQL db_add failed: %s", str(e))
        conn.rollback()
        raise

def db_query(ids, start, end, limit=1000000, sketch=None):
    """Query readings from the 'data' table."""
    conn = _get_conn()
    results = []
    try:
        with conn.cursor() as cur:
            for sid in ids:
                cur.execute(
                    "SELECT time, value FROM data WHERE sid = %s AND time >= %s AND time <= %s ORDER BY time ASC LIMIT %s",
                    (sid, start, end, limit)
                )
                rows = cur.fetchall()
                results.append(np.array(rows, dtype=np.float64) if rows else np.empty((0, 2)))
    finally:
        conn.close()
    return results

def db_prev(ids, start, n=1):
    """Get previous N readings."""
    conn = _get_conn()
    results = []
    try:
        with conn.cursor() as cur:
            for sid in ids:
                cur.execute(
                    "SELECT time, value FROM data WHERE sid = %s AND time <= %s ORDER BY time DESC LIMIT %s",
                    (sid, start, n)
                )
                rows = cur.fetchall()
                # reverse so they are in ascending time order
                rows.reverse()
                results.append(np.array(rows, dtype=np.float64) if rows else np.empty((0, 2)))
    finally:
        conn.close()
    return results

def db_next(ids, start, n=1):
    """Get next N readings."""
    conn = _get_conn()
    results = []
    try:
        with conn.cursor() as cur:
            for sid in ids:
                cur.execute(
                    "SELECT time, value FROM data WHERE sid = %s AND time >= %s ORDER BY time ASC LIMIT %s",
                    (sid, start, n)
                )
                rows = cur.fetchall()
                results.append(np.array(rows, dtype=np.float64) if rows else np.empty((0, 2)))
    finally:
        conn.close()
    return results

def db_del(conn, sid, start, end):
    """Delete readings in a range."""
    try:
        with conn.cursor() as cur:
            cur.execute(
                "DELETE FROM data WHERE sid = %s AND time >= %s AND time <= %s",
                (sid, start, end)
            )
            conn.commit()
    except Exception as e:
        logging.error("SQL db_del failed: %s", str(e))
        conn.rollback()
        raise
