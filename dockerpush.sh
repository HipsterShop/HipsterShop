#!/bin/bash
docker build -t cazzzzz/trf-adservice-hipster:latest ./src/adservice
docker push cazzzzz/trf-adservice-hipster:latest
docker build -t cazzzzz/trf-assistantservice-hipster:latest ./src/assistantservice
docker push cazzzzz/trf-assistantservice-hipster:latest
docker build -t cazzzzz/trf-authservice-hipster:latest ./src/authservice
docker push cazzzzz/trf-authservice-hipster:latest
docker build -t cazzzzz/trf-cartservice-hipster:latest ./src/cartservice/src
docker push cazzzzz/trf-cartservice-hipster:latest
docker build -t cazzzzz/trf-checkoutservice-hipster:latest ./src/checkoutservice
docker push cazzzzz/trf-checkoutservice-hipster:latest
docker build -t cazzzzz/trf-currencyservice-hipster:latest ./src/currencyservice
docker push cazzzzz/trf-currencyservice-hipster:latest
docker build -t cazzzzz/trf-emailservice-hipster:latest ./src/emailservice
docker push cazzzzz/trf-emailservice-hipster:latest
docker build -t cazzzzz/trf-frontend-hipster:latest ./src/frontend
docker push cazzzzz/trf-frontend-hipster:latest
docker build -t cazzzzz/trf-paymentservice-hipster:latest ./src/paymentservice
docker push cazzzzz/trf-paymentservice-hipster:latest
docker build -t cazzzzz/trf-productcatalogservice-hipster:latest ./src/productcatalogservice
docker push cazzzzz/trf-productcatalogservice-hipster:latest
docker build -t cazzzzz/trf-recommendationservice-hipster:latest ./src/recommendationservice
docker push cazzzzz/trf-recommendationservice-hipster:latest
docker build -t cazzzzz/trf-shippingservice-hipster:latest ./src/shippingservice
docker push cazzzzz/trf-shippingservice-hipster:latest
