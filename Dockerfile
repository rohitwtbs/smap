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
    && rm -rf /var/lib/apt/lists/*

COPY . /opt/smap

RUN pip install --upgrade pip setuptools wheel && \
    pip install -r python/requirements.txt

RUN pip install 'bandit<1.7' 'safety<2.0'

CMD ["twistd", "-n", "smap-archiver", "python/conf/archiver.ini"]
