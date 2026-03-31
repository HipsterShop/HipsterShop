using System;
using System.Linq;
using System.Text.Json;
using System.Threading.Tasks;
using Npgsql;
using cartservice.models;

namespace cartservice.cartstore
{
    public class PostgresCartStore : ICartStore
    {
        private readonly string _connectionString;

        public PostgresCartStore(string connectionString)
        {
            _connectionString = connectionString;
        }

        public async Task AddItemAsync(string userId, string productId, int quantity)
        {
            Console.WriteLine($"AddItemAsync called with userId={userId}, productId={productId}, quantity={quantity}");

            try
            {
                using var conn = new NpgsqlConnection(_connectionString);
                await conn.OpenAsync();

                Cart cart = new Cart { UserId = userId };
                
                using (var cmd = new NpgsqlCommand("SELECT cart_data FROM carts WHERE user_id = @userId", conn))
                {
                    cmd.Parameters.AddWithValue("userId", userId);
                    using var reader = await cmd.ExecuteReaderAsync();
                    if (await reader.ReadAsync())
                    {
                        var json = reader.GetString(0);
                        cart = JsonSerializer.Deserialize<Cart>(json) ?? new Cart { UserId = userId };
                    }
                }

                var existingItem = cart.Items.SingleOrDefault(i => i.ProductId == productId);
                if (existingItem == null)
                {
                    cart.Items.Add(new CartItem { ProductId = productId, Quantity = quantity });
                }
                else
                {
                    existingItem.Quantity += quantity;
                }

                var cartJson = JsonSerializer.Serialize(cart);
                using (var cmd = new NpgsqlCommand(@"
                    INSERT INTO carts (user_id, cart_data) VALUES (@userId, @cartData::bytea)
                    ON CONFLICT (user_id) DO UPDATE SET cart_data = EXCLUDED.cart_data", conn))
                {
                    cmd.Parameters.AddWithValue("userId", userId);
                    cmd.Parameters.AddWithValue("cartData", System.Text.Encoding.UTF8.GetBytes(cartJson));
                    await cmd.ExecuteNonQueryAsync();
                }
            }
            catch (Exception ex)
            {
                throw new Exception($"Can't access cart storage. {ex}");
            }
        }

        public async Task EmptyCartAsync(string userId)
        {
            Console.WriteLine($"EmptyCartAsync called with userId={userId}");
            try
            {
                using var conn = new NpgsqlConnection(_connectionString);
                await conn.OpenAsync();
                using var cmd = new NpgsqlCommand("DELETE FROM carts WHERE user_id = @userId", conn);
                cmd.Parameters.AddWithValue("userId", userId);
                await cmd.ExecuteNonQueryAsync();
            }
            catch (Exception ex)
            {
                throw new Exception($"Can't access cart storage. {ex}");
            }
        }

        public async Task<Cart> GetCartAsync(string userId)
        {
            Console.WriteLine($"GetCartAsync called with userId={userId}");
            try
            {
                using var conn = new NpgsqlConnection(_connectionString);
                await conn.OpenAsync();
                using var cmd = new NpgsqlCommand("SELECT cart_data FROM carts WHERE user_id = @userId", conn);
                cmd.Parameters.AddWithValue("userId", userId);
                using var reader = await cmd.ExecuteReaderAsync();
                if (await reader.ReadAsync())
                {
                    var bytes = (byte[])reader["cart_data"];
                    var json = System.Text.Encoding.UTF8.GetString(bytes);
                    return JsonSerializer.Deserialize<Cart>(json) ?? new Cart { UserId = userId };
                }
                return new Cart { UserId = userId };
            }
            catch (Exception ex)
            {
                throw new Exception($"Can't access cart storage. {ex}");
            }
        }

        public bool Ping()
        {
            try
            {
                using var conn = new NpgsqlConnection(_connectionString);
                conn.Open();
                return true;
            }
            catch (Exception)
            {
                return false;
            }
        }
    }
}
