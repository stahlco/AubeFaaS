FROM scratch

ADD blob.tar.gz /

EXPOSE 8000

# Create app directory
WORKDIR /usr/src/app

COPY fn/* ./
RUN uv pip install -r requirements.txt --user

ENV PYTHONUNBUFFERED=1
CMD [ "python3", "functionhandler.py" ]