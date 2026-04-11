
using System;
using System.Linq;
using System.Text.Json;
using System.Threading.Tasks;
using Microsoft.Extensions.Caching.Distributed;
using cartservice.models;

namespace cartservice.cartstore
{
    public class RedisCartStore : ICartStore
    {
        private readonly IDistributedCache _cache;

        public RedisCartStore(IDistributedCache cache)
        {
            _cache = cache;
        }

        public async Task AddItemAsync(string userId, string productId, int quantity)
        {
            Console.WriteLine($"AddItemAsync called with userId={userId}, productId={productId}, quantity={quantity}");

            try
            {
                Cart cart;
                var value = await _cache.GetAsync(userId);
                if (value == null)
                {
                    cart = new Cart();
                    cart.UserId = userId;
                    cart.Items.Add(new CartItem { ProductId = productId, Quantity = quantity });
                }
                else
                {
                    var json = System.Text.Encoding.UTF8.GetString(value);
                    cart = JsonSerializer.Deserialize<Cart>(json) ?? new Cart { UserId = userId };
                    var existingItem = cart.Items.SingleOrDefault(i => i.ProductId == productId);
                    if (existingItem == null)
                    {
                        cart.Items.Add(new CartItem { ProductId = productId, Quantity = quantity });
                    }
                    else
                    {
                        existingItem.Quantity += quantity;
                    }
                }
                var cartJson = JsonSerializer.Serialize(cart);
                await _cache.SetAsync(userId, System.Text.Encoding.UTF8.GetBytes(cartJson));
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
                var cart = new Cart();
                var cartJson = JsonSerializer.Serialize(cart);
                await _cache.SetAsync(userId, System.Text.Encoding.UTF8.GetBytes(cartJson));
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
                var value = await _cache.GetAsync(userId);

                if (value != null)
                {
                    var json = System.Text.Encoding.UTF8.GetString(value);
                    return JsonSerializer.Deserialize<Cart>(json) ?? new Cart();
                }

                return new Cart();
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
                return true;
            }
            catch (Exception)
            {
                return false;
            }
        }
    }
}
