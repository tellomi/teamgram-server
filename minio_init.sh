#!/bin/bash
sleep 30
mc alias set minio http://minio:9000 minio miniostorage
mc mb minio/tellomi-attachments
mc mb minio/tellomi-public-media
mc mb minio/tellomi-backups
exit
