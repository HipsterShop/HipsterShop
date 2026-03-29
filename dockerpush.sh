#!/bin/bash
docker build -t cazzzzz/tr-adservice-hipster:latest ./src/adservice
docker push cazzzzz/tr-adservice-hipster:latest
docker build -t cazzzzz/tr-cartservice-hipster:latest ./src/cartservice/src
docker push cazzzzz/tr-cartservice-hipster:latest
docker build -t cazzzzz/tr-checkoutservice-hipster:latest ./src/checkoutservice
docker push cazzzzz/tr-checkoutservice-hipster:latest
docker build -t cazzzzz/tr-currencyservice-hipster:latest ./src/currencyservice
docker push cazzzzz/tr-currencyservice-hipster:latest
docker build -t cazzzzz/tr-emailservice-hipster:latest ./src/emailservice
docker push cazzzzz/tr-emailservice-hipster:latest
docker build -t cazzzzz/tr-frontend-hipster:latest ./src/frontend
docker push cazzzzz/tr-frontend-hipster:latest
docker build -t cazzzzz/tr-loadgenerator-hipster:latest ./src/loadgenerator
docker push cazzzzz/tr-loadgenerator-hipster:latest
docker build -t cazzzzz/tr-paymentservice-hipster:latest ./src/paymentservice
docker push cazzzzz/tr-paymentservice-hipster:latest
docker build -t cazzzzz/tr-productcatalogservice-hipster:latest ./src/productcatalogservice
docker push cazzzzz/tr-productcatalogservice-hipster:latest
docker build -t cazzzzz/tr-recommendationservice-hipster:latest ./src/recommendationservice
docker push cazzzzz/tr-recommendationservice-hipster:latest
docker build -t cazzzzz/tr-shippingservice-hipster:latest ./src/shippingservice
docker push cazzzzz/tr-shippingservice-hipster:latest
