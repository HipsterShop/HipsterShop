/*
 *
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 */

package hipstershop;

import com.google.common.collect.ImmutableListMultimap;
import com.google.common.collect.Iterables;
import com.sun.net.httpserver.HttpServer;
import com.sun.net.httpserver.HttpExchange;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.node.ObjectNode;
import com.fasterxml.jackson.databind.node.ArrayNode;

import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.InetSocketAddress;
import java.nio.charset.StandardCharsets;
import java.util.*;
import org.apache.logging.log4j.LogManager;
import org.apache.logging.log4j.Logger;

public final class AdService {

  private static final Logger logger = LogManager.getLogger(AdService.class);
  private static final ObjectMapper mapper = new ObjectMapper();

  @SuppressWarnings("FieldCanBeLocal")
  private static int MAX_ADS_TO_SERVE = 2;

  private HttpServer server;

  private static final AdService service = new AdService();

  // Simple Ad POJO
  public static class Ad {
    public String redirect_url;
    public String text;

    public Ad(String redirectUrl, String text) {
      this.redirect_url = redirectUrl;
      this.text = text;
    }
  }

  // Request POJO
  public static class AdRequest {
    public List<String> context_keys;
  }

  private void start() throws IOException {
    int port = Integer.parseInt(System.getenv().getOrDefault("PORT", "9555"));

    server = HttpServer.create(new InetSocketAddress(port), 0);
    server.createContext("/ads", this::handleGetAds);
    server.createContext("/_healthz", this::handleHealthCheck);
    server.setExecutor(null);
    server.start();

    logger.info("Ad Service started, listening on " + port);
    Runtime.getRuntime()
        .addShutdownHook(
            new Thread(
                () -> {
                  System.err.println(
                      "*** shutting down HTTP ads server since JVM is shutting down");
                  AdService.this.stop();
                  System.err.println("*** server shut down");
                }));
  }

  private void stop() {
    if (server != null) {
      server.stop(0);
    }
  }

  private void handleGetAds(HttpExchange exchange) throws IOException {
    if (!"POST".equals(exchange.getRequestMethod())) {
      sendResponse(exchange, 405, "Method Not Allowed");
      return;
    }

    try {
      InputStream is = exchange.getRequestBody();
      String body = new String(is.readAllBytes(), StandardCharsets.UTF_8);
      AdRequest req = mapper.readValue(body, AdRequest.class);

      List<Ad> allAds = new ArrayList<>();
      logger.info("received ad request (context_words=" + (req.context_keys != null ? req.context_keys : "[]") + ")");

      if (req.context_keys != null && !req.context_keys.isEmpty()) {
        for (String key : req.context_keys) {
          Collection<Ad> ads = getAdsByCategory(key);
          allAds.addAll(ads);
        }
      } else {
        allAds = getRandomAds();
      }
      if (allAds.isEmpty()) {
        allAds = getRandomAds();
      }

      ObjectNode responseNode = mapper.createObjectNode();
      ArrayNode adsArray = responseNode.putArray("ads");
      for (Ad ad : allAds) {
        ObjectNode adNode = mapper.createObjectNode();
        adNode.put("redirect_url", ad.redirect_url);
        adNode.put("text", ad.text);
        adsArray.add(adNode);
      }

      String json = mapper.writeValueAsString(responseNode);
      exchange.getResponseHeaders().set("Content-Type", "application/json");
      sendResponse(exchange, 200, json);

    } catch (Exception e) {
      logger.error("GetAds Failed", e);
      sendResponse(exchange, 500, "{\"error\": \"" + e.getMessage() + "\"}");
    }
  }

  private void handleHealthCheck(HttpExchange exchange) throws IOException {
    sendResponse(exchange, 200, "ok");
  }

  private void sendResponse(HttpExchange exchange, int statusCode, String response) throws IOException {
    byte[] bytes = response.getBytes(StandardCharsets.UTF_8);
    exchange.sendResponseHeaders(statusCode, bytes.length);
    OutputStream os = exchange.getResponseBody();
    os.write(bytes);
    os.close();
  }

  private static final ImmutableListMultimap<String, Ad> adsMap = createAdsMap();

  private Collection<Ad> getAdsByCategory(String category) {
    return adsMap.get(category);
  }

  private static final Random random = new Random();

  private List<Ad> getRandomAds() {
    List<Ad> ads = new ArrayList<>(MAX_ADS_TO_SERVE);
    Collection<Ad> allAds = adsMap.values();
    for (int i = 0; i < MAX_ADS_TO_SERVE; i++) {
      ads.add(Iterables.get(allAds, random.nextInt(allAds.size())));
    }
    return ads;
  }

  private static AdService getInstance() {
    return service;
  }

  private void blockUntilShutdown() throws InterruptedException {
    // Keep the main thread alive
    Thread.currentThread().join();
  }

  private static ImmutableListMultimap<String, Ad> createAdsMap() {
    Ad hairdryer = new Ad("/product/2ZYFJ3GM2N", "Hairdryer for sale. 50% off.");
    Ad tankTop = new Ad("/product/66VCHSJNUP", "Tank top for sale. 20% off.");
    Ad candleHolder = new Ad("/product/0PUK6V6EV0", "Candle holder for sale. 30% off.");
    Ad bambooGlassJar = new Ad("/product/9SIQT8TOJO", "Bamboo glass jar for sale. 10% off.");
    Ad watch = new Ad("/product/1YMWWN1N4O", "Watch for sale. Buy one, get second kit for free");
    Ad mug = new Ad("/product/6E92ZMYYFZ", "Mug for sale. Buy two, get third one for free");
    Ad loafers = new Ad("/product/L9ECAV7KIM", "Loafers for sale. Buy one, get second one for free");

    return ImmutableListMultimap.<String, Ad>builder()
        .putAll("clothing", tankTop)
        .putAll("accessories", watch)
        .putAll("footwear", loafers)
        .putAll("hair", hairdryer)
        .putAll("decor", candleHolder)
        .putAll("kitchen", bambooGlassJar, mug)
        .build();
  }

  /** Main launches the server from the command line. */
  public static void main(String[] args) throws IOException, InterruptedException {
    logger.info("AdService starting.");
    final AdService service = AdService.getInstance();
    service.start();
    service.blockUntilShutdown();
  }
}
