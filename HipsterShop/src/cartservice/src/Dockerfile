FROM mcr.microsoft.com/dotnet/sdk:10.0.100-noble@sha256:c7445f141c04f1a6b454181bd098dcfa606c61ba0bd213d0a702489e5bd4cd71 AS builder
WORKDIR /app
COPY cartservice.csproj .
RUN dotnet restore cartservice.csproj
COPY . .
RUN dotnet publish cartservice.csproj \
    -c release \
    -o /cartservice
FROM mcr.microsoft.com/dotnet/aspnet:10.0-noble
WORKDIR /app
COPY --from=builder /cartservice .
EXPOSE 7070
ENV DOTNET_EnableDiagnostics=0 \
    ASPNETCORE_HTTP_PORTS=7070
USER 1000
ENTRYPOINT ["dotnet", "cartservice.dll"]
