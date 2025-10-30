FROM python:3.11-alpine

EXPOSE 8001

RUN pip install websockets

COPY functionhandler.py /usr/src/app/

WORKDIR /usr/src/app

COPY fn ./fn/

RUN pip install -r fn/requirements.txt

CMD ["python3", "functionhandler.py"]