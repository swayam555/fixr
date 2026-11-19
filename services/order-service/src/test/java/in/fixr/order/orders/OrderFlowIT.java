package in.fixr.order.orders;

import static org.assertj.core.api.Assertions.assertThat;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.get;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.post;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.jsonPath;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

import com.jayway.jsonpath.JsonPath;
import in.fixr.order.AbstractIT;
import in.fixr.order.outbox.PublishFaults;
import java.time.Duration;
import java.util.ArrayList;
import java.util.List;
import java.util.Properties;
import java.util.UUID;
import java.util.function.Predicate;
import org.apache.kafka.clients.consumer.ConsumerConfig;
import org.apache.kafka.clients.consumer.ConsumerRecord;
import org.apache.kafka.clients.consumer.KafkaConsumer;
import org.apache.kafka.common.serialization.StringDeserializer;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.http.MediaType;
import org.springframework.jdbc.core.simple.JdbcClient;
import org.springframework.kafka.core.KafkaTemplate;

class OrderFlowIT extends AbstractIT {
    private static final double KORA_LAT = 12.9352, KORA_LNG = 77.6245;

    @Autowired KafkaTemplate<String, String> kafka;
    @Autowired PublishFaults faults;
    @Autowired JdbcClient jdbc;

    private String address(Session s, double lat, double lng) throws Exception {
        String body = mvc.perform(post("/api/addresses").header("Authorization", s.bearer()).contentType(MediaType.APPLICATION_JSON)
                        .content("{\"label\":\"Home\",\"line1\":\"5th Block\",\"society\":\"Prestige Acropolis\",\"flat\":\"B-1204\","
                                + "\"lat\":" + lat + ",\"lng\":" + lng + "}"))
                .andExpect(status().isCreated()).andReturn().getResponse().getContentAsString();
        return JsonPath.read(body, "$.id");
    }

    private String book(Session s, String addressId, String key, int expect) throws Exception {
        return mvc.perform(post("/api/orders").header("Authorization", s.bearer()).header("Idempotency-Key", key)
                        .contentType(MediaType.APPLICATION_JSON)
                        .content("{\"serviceId\":\"tap-leak\",\"addressId\":\"" + addressId + "\",\"notes\":\"Kitchen tap\"}"))
                .andExpect(status().is(expect)).andReturn().getResponse().getContentAsString();
    }

    private String act(Session s, String orderId, String action, String body, int expect) throws Exception {
        return mvc.perform(post("/api/orders/" + orderId + "/" + action).header("Authorization", s.bearer())
                        .contentType(MediaType.APPLICATION_JSON).content(body))
                .andExpect(status().is(expect)).andReturn().getResponse().getContentAsString();
    }

    private void dispatchResult(String eventId, String type, String orderId, String proId) {
        String v = "{\"eventId\":\"" + eventId + "\",\"type\":\"" + type + "\",\"orderId\":\"" + orderId + "\""
                + (proId == null ? "" : ",\"proId\":\"" + proId + "\",\"etaSec\":420") + ",\"atMs\":" + System.currentTimeMillis() + "}";
        kafka.send("dispatch.results", orderId, v).join();
    }

    /** Reads order.events for one order until the predicate holds or the timeout passes. */
    private List<String> orderEvents(String orderId, Predicate<List<String>> until) {
        Properties p = new Properties();
        p.put(ConsumerConfig.BOOTSTRAP_SERVERS_CONFIG, kafka.getProducerFactory().getConfigurationProperties().get("bootstrap.servers"));
        p.put(ConsumerConfig.GROUP_ID_CONFIG, "it-" + UUID.randomUUID());
        p.put(ConsumerConfig.AUTO_OFFSET_RESET_CONFIG, "earliest");
        List<String> types = new ArrayList<>();
        try (var c = new KafkaConsumer<>(p, new StringDeserializer(), new StringDeserializer())) {
            c.subscribe(List.of("order.events"));
            long deadline = System.currentTimeMillis() + 20_000;
            while (System.currentTimeMillis() < deadline) {
                for (ConsumerRecord<String, String> r : c.poll(Duration.ofMillis(300))) {
                    if (orderId.equals(r.key())) types.add(JsonPath.read(r.value(), "$.type"));
                }
                if (until.test(types)) {
                    // drain a little longer so a late duplicate would be caught
                    for (ConsumerRecord<String, String> r : c.poll(Duration.ofSeconds(2))) {
                        if (orderId.equals(r.key())) types.add(JsonPath.read(r.value(), "$.type"));
                    }
                    break;
                }
            }
        }
        return types;
    }

    private String orderStatus(Session s, String orderId) throws Exception {
        return JsonPath.read(mvc.perform(get("/api/orders/" + orderId).header("Authorization", s.bearer()))
                .andReturn().getResponse().getContentAsString(), "$.status");
    }

    private void awaitStatus(Session s, String orderId, String want) throws Exception {
        long deadline = System.currentTimeMillis() + 15_000;
        while (!want.equals(orderStatus(s, orderId))) {
            if (System.currentTimeMillis() > deadline) throw new AssertionError("order never became " + want);
            Thread.sleep(100);
        }
    }

    @Test
    void catalogueAndZoneCheckArePublic() throws Exception {
        mvc.perform(get("/api/catalogue")).andExpect(status().isOk())
                .andExpect(jsonPath("$[0].id").value("plumbing"))
                .andExpect(jsonPath("$[0].services[0].id").value("tap-leak"));
        mvc.perform(get("/api/zones/serviceable").param("lat", "" + KORA_LAT).param("lng", "" + KORA_LNG))
                .andExpect(jsonPath("$.serviceable").value(true))
                .andExpect(jsonPath("$.zoneId").value("blr-koramangala"));
        mvc.perform(get("/api/zones/serviceable").param("lat", "13.2").param("lng", "77.3"))
                .andExpect(jsonPath("$.serviceable").value(false));
    }

    @Test
    void bookingEmitsExactlyOneCreatedEventEvenWhenThePublishIsRetried() throws Exception {
        Session c = login("CUSTOMER");
        String addr = address(c, KORA_LAT, KORA_LNG);
        faults.failNext(1);
        String key = UUID.randomUUID().toString();
        String created = book(c, addr, key, 201);
        String orderId = JsonPath.read(created, "$.id");
        assertThat((String) JsonPath.read(created, "$.status")).isEqualTo("SEARCHING");
        assertThat((String) JsonPath.read(created, "$.startCode")).matches("\\d{4}");

        String replay = book(c, addr, key, 200);
        assertThat((String) JsonPath.read(replay, "$.id")).isEqualTo(orderId);

        List<String> events = orderEvents(orderId, t -> t.contains("ORDER_SEARCHING"));
        assertThat(events).containsExactly("ORDER_CREATED", "ORDER_SEARCHING");
        int attempts = jdbc.sql("SELECT max(attempts) FROM outbox WHERE msg_key = ?").param(orderId).query(Integer.class).single();
        assertThat(attempts).as("first publish was failed on purpose and retried").isEqualTo(2);
    }

    @Test
    void fullJobLifecycleWithDuplicateDispatchResult() throws Exception {
        Session c = login("CUSTOMER");
        Session pro = login("PRO");
        String orderId = JsonPath.read(book(c, address(c, KORA_LAT, KORA_LNG), UUID.randomUUID().toString(), 201), "$.id");

        String accept = UUID.randomUUID().toString();
        dispatchResult(accept, "ACCEPTED", orderId, pro.userId());
        dispatchResult(accept, "ACCEPTED", orderId, pro.userId());
        awaitStatus(c, orderId, "ASSIGNED");

        String asPro = mvc.perform(get("/api/orders/" + orderId).header("Authorization", pro.bearer()))
                .andExpect(status().isOk()).andReturn().getResponse().getContentAsString();
        assertThat((Object) JsonPath.read(asPro, "$.startCode")).as("pros never see the start code").isNull();
        int version = JsonPath.read(asPro, "$.version");

        act(pro, orderId, "en-route", "{\"version\":" + version + "}", 200);
        act(pro, orderId, "arrived", "{\"version\":" + version + "}", 409);
        act(pro, orderId, "arrived", "{}", 200);
        String code = JsonPath.read(mvc.perform(get("/api/orders/" + orderId).header("Authorization", c.bearer()))
                .andReturn().getResponse().getContentAsString(), "$.startCode");
        String wrong = code.equals("0000") ? "1111" : "0000";
        act(pro, orderId, "start", "{\"code\":\"" + wrong + "\"}", 400);
        act(pro, orderId, "start", "{\"code\":\"" + code + "\"}", 200);
        act(pro, orderId, "complete", "{}", 200);
        act(c, orderId, "cancel", "{}", 409);

        List<String> events = orderEvents(orderId, t -> t.contains("ORDER_COMPLETED"));
        assertThat(events).containsExactly("ORDER_CREATED", "ORDER_SEARCHING", "ORDER_ASSIGNED", "ORDER_EN_ROUTE",
                "ORDER_ARRIVED", "ORDER_IN_PROGRESS", "ORDER_COMPLETED");

        mvc.perform(get("/api/orders/" + orderId + "/timeline").header("Authorization", c.bearer()))
                .andExpect(jsonPath("$.length()").value(6))
                .andExpect(jsonPath("$[1].actor").value("DISPATCH"));
    }

    @Test
    void lateAcceptAfterCancelReleasesThePro() throws Exception {
        Session c = login("CUSTOMER");
        Session pro = login("PRO");
        String orderId = JsonPath.read(book(c, address(c, KORA_LAT, KORA_LNG), UUID.randomUUID().toString(), 201), "$.id");
        act(c, orderId, "cancel", "{\"reason\":\"Fixed it myself\"}", 200);
        dispatchResult(UUID.randomUUID().toString(), "ACCEPTED", orderId, pro.userId());

        List<String> events = orderEvents(orderId, t -> t.contains("ORDER_ASSIGN_REJECTED"));
        assertThat(events).containsExactly("ORDER_CREATED", "ORDER_SEARCHING", "ORDER_CANCELLED", "ORDER_ASSIGN_REJECTED");
        assertThat(orderStatus(c, orderId)).isEqualTo("CANCELLED");
    }

    @Test
    void proDropRedispatchesAndExcludesThem() throws Exception {
        Session c = login("CUSTOMER");
        Session pro = login("PRO");
        String orderId = JsonPath.read(book(c, address(c, KORA_LAT, KORA_LNG), UUID.randomUUID().toString(), 201), "$.id");
        dispatchResult(UUID.randomUUID().toString(), "ACCEPTED", orderId, pro.userId());
        awaitStatus(c, orderId, "ASSIGNED");
        act(pro, orderId, "drop", "{\"reason\":\"Bike punctured\"}", 200);
        assertThat(orderStatus(c, orderId)).isEqualTo("SEARCHING");
        mvc.perform(get("/api/orders/" + orderId).header("Authorization", pro.bearer())).andExpect(status().isNotFound());

        List<String> events = orderEvents(orderId, t -> t.contains("ORDER_REDISPATCH"));
        assertThat(events).endsWith("ORDER_REDISPATCH");
        String payloadPro = jdbc.sql("SELECT payload->'payload'->>'proId' FROM outbox WHERE msg_key = ? AND event_type = 'ORDER_REDISPATCH'")
                .param(orderId).query(String.class).single();
        assertThat(payloadPro).as("redispatch names the pro to exclude").isEqualTo(pro.userId());
    }

    @Test
    void rejectsUnserviceableAddressesAndOtherPeoplesOrders() throws Exception {
        Session c = login("CUSTOMER");
        String outside = address(c, 13.2, 77.3);
        String body = book(c, outside, UUID.randomUUID().toString(), 400);
        assertThat((String) JsonPath.read(body, "$.code")).isEqualTo("NOT_SERVICEABLE");

        String orderId = JsonPath.read(book(c, address(c, KORA_LAT, KORA_LNG), UUID.randomUUID().toString(), 201), "$.id");
        Session stranger = login("CUSTOMER");
        mvc.perform(get("/api/orders/" + orderId).header("Authorization", stranger.bearer())).andExpect(status().isNotFound());
        mvc.perform(post("/api/orders").header("Authorization", c.bearer()).contentType(MediaType.APPLICATION_JSON)
                        .content("{\"serviceId\":\"tap-leak\",\"addressId\":\"" + UUID.randomUUID() + "\"}"))
                .andExpect(status().isBadRequest())
                .andExpect(jsonPath("$.code").value("IDEMPOTENCY_KEY_REQUIRED"));
    }
}
