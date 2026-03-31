using System.Collections.Generic;
using System.Text.Json.Serialization;

namespace cartservice.models
{
    public class CartItem
    {
        [JsonPropertyName("productId")]
        public string ProductId { get; set; } = "";

        [JsonPropertyName("quantity")]
        public int Quantity { get; set; }
    }

    public class Cart
    {
        [JsonPropertyName("userId")]
        public string UserId { get; set; } = "";

        [JsonPropertyName("items")]
        public List<CartItem> Items { get; set; } = new List<CartItem>();
    }

    public class AddItemRequest
    {
        [JsonPropertyName("userId")]
        public string UserId { get; set; } = "";

        [JsonPropertyName("item")]
        public CartItem Item { get; set; } = new CartItem();
    }

    public class GetCartRequest
    {
        [JsonPropertyName("userId")]
        public string UserId { get; set; } = "";
    }

    public class EmptyCartRequest
    {
        [JsonPropertyName("userId")]
        public string UserId { get; set; } = "";
    }
}
