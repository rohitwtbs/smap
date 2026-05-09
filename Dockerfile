FROM python:2.7-slim

ENV PYTHONUNBUFFERED=1 \
    PYTHONPATH=/opt/smap/python

WORKDIR /opt/smap

RUN sed -i 's|deb.debian.org|archive.debian.org|g' /etc/apt/sources.list && \
    sed -i 's|security.debian.org|archive.debian.org|g' /etc/apt/sources.list && \
    sed -i '/buster-updates/d' /etc/apt/sources.list && \
    apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    python-dev \
    libssl-dev \
    libpq-dev \
    python-scipy \
    python-numpy \
    python-psycopg2 \
    git autoconf automake libtool swig check pkg-config \
    libdb-dev libprotobuf-c-dev protobuf-c-compiler zlib1g-dev \
    && rm -rf /var/lib/apt/lists/*

# Install numpy via pip because the readingdb python bindings require it during setup.py
RUN pip install --upgrade pip setuptools wheel && \
    pip install numpy==1.16.6

# Build readingdb
RUN git clone https://github.com/stevedh/readingdb.git /opt/readingdb && \
    cd /opt/readingdb && \
    autoreconf --install && \
    ./configure --prefix=/usr && \
    make && \
    make install && \
    cd iface_bin && \
    make && \
    make install

COPY . /opt/smap

RUN pip install -r python/requirements.txt psycopg2-binary==2.8.6 simplejson ordereddict Twisted==20.3.0 Automat==20.2.0

RUN pip install 'bandit<1.7' 'safety<2.0'

RUN useradd -r readingdb && \
    mkdir -p /var/lib/readingdb && \
    chown readingdb:readingdb /var/lib/readingdb

RUN chmod +x /opt/smap/start.sh

CMD ["bash", "/opt/smap/start.sh"]
