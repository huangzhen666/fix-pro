FROM python:3.13.7-alpine
WORKDIR /probe
COPY probe.py .
USER 65534:65534
CMD ["python", "/probe/probe.py"]
