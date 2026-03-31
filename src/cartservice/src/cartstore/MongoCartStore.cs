using System;
using System.Collections.Generic;
using System.Threading.Tasks;
using cartservice.models;
using MongoDB.Bson;
using MongoDB.Bson.Serialization.Attributes;
using MongoDB.Driver;

namespace cartservice.cartstore
{
    /// <summary>
    /// MongoDB-backed cart store. Each user's cart is a single document
    /// keyed by userId. Connects to the MongoDB replica set (rs0).
    /// </summary>
    public class MongoCartStore : ICartStore
    {
        private readonly IMongoCollection<CartDocument> _collection;

        public MongoCartStore(string connectionString, string databaseName)
        {
            var client = new MongoClient(connectionString);
            var dbName = string.IsNullOrWhiteSpace(databaseName) ? "cart_db" : databaseName;
            var database = client.GetDatabase(dbName);
            _collection = database.GetCollection<CartDocument>("carts");

            // UserId is the BSON _id field, which already has MongoDB's built-in unique index.
        }

        public async Task AddItemAsync(string userId, string productId, int quantity)
        {
            // Use upsert + array update: if item exists, increment quantity; else push new item
            var filter = Builders<CartDocument>.Filter.Eq(c => c.UserId, userId);

            // Try to increment quantity if the product is already in the cart
            var existingFilter = Builders<CartDocument>.Filter.And(
                filter,
                Builders<CartDocument>.Filter.ElemMatch(c => c.Items, i => i.ProductId == productId)
            );
            var increment = Builders<CartDocument>.Update
                .Inc("items.$.quantity", quantity)
                .Set(c => c.UpdatedAt, DateTime.UtcNow);

            var result = await _collection.UpdateOneAsync(existingFilter, increment);

            if (result.MatchedCount == 0)
            {
                // Product not in cart yet — push it
                var push = Builders<CartDocument>.Update
                    .Push(c => c.Items, new CartItemDocument { ProductId = productId, Quantity = quantity })
                    .SetOnInsert(c => c.UserId, userId)
                    .Set(c => c.UpdatedAt, DateTime.UtcNow);

                await _collection.UpdateOneAsync(filter, push, new UpdateOptions { IsUpsert = true });
            }
        }

        public async Task EmptyCartAsync(string userId)
        {
            var filter = Builders<CartDocument>.Filter.Eq(c => c.UserId, userId);
            var update = Builders<CartDocument>.Update
                .Set(c => c.Items, new List<CartItemDocument>())
                .Set(c => c.UpdatedAt, DateTime.UtcNow);
            await _collection.UpdateOneAsync(filter, update, new UpdateOptions { IsUpsert = true });
        }

        public async Task<Cart> GetCartAsync(string userId)
        {
            var filter = Builders<CartDocument>.Filter.Eq(c => c.UserId, userId);
            var doc = await _collection.Find(filter).FirstOrDefaultAsync();

            var cart = new Cart { UserId = userId };
            if (doc?.Items != null)
            {
                foreach (var item in doc.Items)
                {
                    cart.Items.Add(new CartItem { ProductId = item.ProductId, Quantity = item.Quantity });
                }
            }
            return cart;
        }

        public bool Ping()
        {
            try
            {
                _collection.Database.Client.GetDatabase("admin")
                    .RunCommand<BsonDocument>(new BsonDocument("ping", 1));
                return true;
            }
            catch
            {
                return false;
            }
        }
    }

    // Internal MongoDB document model
    internal class CartDocument
    {
        [BsonId]
        [BsonRepresentation(BsonType.String)]
        public string UserId { get; set; }
        public List<CartItemDocument> Items { get; set; } = new();
        public DateTime UpdatedAt { get; set; } = DateTime.UtcNow;
    }

    internal class CartItemDocument
    {
        public string ProductId { get; set; }
        public int Quantity { get; set; }
    }
}
