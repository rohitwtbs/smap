FROM python:2.7-slim

ENV PYTHONUNBUFFERED=1 \
    PYTHONPATH=/opt/smap/python

WORKDIR /opt/smap

# Use pre-compiled Debian packages for Python 2.7 to avoid slow/failing pip builds.
# Buster is EOL, so we use the archive mirrors.
RUN sed -i 's|deb.debian.org|archive.debian.org|g' /etc/apt/sources.list && \
    sed -i 's|security.debian.org|archive.debian.org|g' /etc/apt/sources.list && \
    sed -i '/buster-updates/d' /etc/apt/sources.list && \
    apt-get -o Acquire::Check-Valid-Until=false update && apt-get install -y --no-install-recommends \
    ca-certificates \
    git \
    postgresql-client \
    python-twisted \
    python-psycopg2 \
    python-dateutil \
    python-configobj \
    python-ply \
    python-numpy \
    python-scipy \
    python-pycurl \
    python-autobahn \
    python-openssl \
    && mkdir -p /etc/smap \
    && rm -rf /var/lib/apt/lists/*

COPY . /opt/smap

# Create VERSION file so setup.py doesn't fail due to missing .git
RUN echo "2.0-docker" > VERSION

# Install remaining small/pure-python dependencies
RUN pip install --upgrade "pip<21.0" "setuptools<45.0" wheel && \
    pip install "lockfile" "avro>=1.6.3"

# Install the smap package
RUN cd python && python setup.py install

# Install security tools for CI/Dev (compat with Python 2.7)
RUN pip install "bandit<1.7.0" "safety<1.10.0"

CMD ["bash"]
