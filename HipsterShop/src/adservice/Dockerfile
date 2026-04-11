FROM eclipse-temurin:24.0.2_12-jdk-noble@sha256:dacac8e9a0df0d2bd24e702b4431132875c249930b70555ebd7ca285b5bee684 AS builder
WORKDIR /app
COPY ["build.gradle", "gradlew", "./"]
COPY gradle gradle
RUN chmod +x gradlew
RUN ./gradlew downloadRepos
COPY . .
RUN chmod +x gradlew
RUN ./gradlew installDist
FROM eclipse-temurin:25.0.2_10-jre-alpine@sha256:f10d6259d0798c1e12179b6bf3b63cea0d6843f7b09c9f9c9c422c50e44379ec
WORKDIR /app
COPY --from=builder /app .
RUN wget -O /opentelemetry-javaagent.jar https://github.com/open-telemetry/opentelemetry-java-instrumentation/releases/latest/download/opentelemetry-javaagent.jar
ENV JAVA_TOOL_OPTIONS="-javaagent:/opentelemetry-javaagent.jar"
EXPOSE 9555
ENTRYPOINT ["/app/build/install/hipstershop/bin/AdService"]
