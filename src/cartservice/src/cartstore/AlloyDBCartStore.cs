
using System;
using Npgsql;
using Microsoft.Extensions.Configuration;
using System.Threading.Tasks;
using Google.Api.Gax.ResourceNames;
using Google.Cloud.SecretManager.V1;
using cartservice.models;
 
namespace cartservice.cartstore
{
    public class AlloyDBCartStore : ICartStore
    {
        private readonly string tableName;
        private readonly string connectionString;

        public AlloyDBCartStore(IConfiguration configuration)
        {
            SecretManagerServiceClient client = SecretManagerServiceClient.Create();
            var projectId = configuration["PROJECT_ID"];
            var secretId = configuration["ALLOYDB_SECRET_NAME"];
            SecretVersionName secretVersionName = new SecretVersionName(projectId, secretId, "latest");

            AccessSecretVersionResponse result = client.AccessSecretVersion(secretVersionName);
            string alloyDBPassword = result.Payload.Data.ToStringUtf8().TrimEnd('\r', '\n');
        
            string alloyDBUser = "postgres";
            string databaseName = configuration["ALLOYDB_DATABASE_NAME"];
            string primaryIPAddress = configuration["ALLOYDB_PRIMARY_IP"];
            connectionString = "Host="          +
                               primaryIPAddress +
                               ";Username="     +
                               alloyDBUser      +
                               ";Password="     +
                               alloyDBPassword  +
                               ";Database="     +
                               databaseName;

            tableName = configuration["ALLOYDB_TABLE_NAME"];
        }


    public async Task AddItemAsync(string userId, string productId, int quantity)
    {
        Console.WriteLine($"AddItemAsync for {userId} called");
        try
        {
            await using var dataSource = NpgsqlDataSource.Create(connectionString);

            var fetchCmd = $"SELECT quantity FROM {tableName} WHERE userID='{userId}' AND productID='{productId}'";
            var currentQuantity = 0;
            var cmdRead = dataSource.CreateCommand(fetchCmd);
            await using (var reader = await cmdRead.ExecuteReaderAsync())
            {
                while (await reader.ReadAsync())
                    currentQuantity += reader.GetInt32(0);
            }

            var totalQuantity = quantity + currentQuantity;

            var insertCmd = $@"
                INSERT INTO {tableName} (userId, productId, quantity)
                VALUES ('{userId}', '{productId}', {totalQuantity})
                ON CONFLICT (userId, productId)
                DO UPDATE SET quantity = {totalQuantity};
            ";

            await using (var cmdInsert = dataSource.CreateCommand(insertCmd))
            {
                await Task.Run(() =>
                {
                    return cmdInsert.ExecuteNonQueryAsync();
                });
            }
        }
        catch (Exception ex)
        {   
            throw new Exception($"Unable to access cart storage due to an internal error. {ex}");
        }
    }


        public async Task<Cart> GetCartAsync(string userId)
        {
            Console.WriteLine($"GetCartAsync called for userId={userId}");
            Cart cart = new() { UserId = userId };
            try
            {
                await using var dataSource = NpgsqlDataSource.Create(connectionString);

                var cartFetchCmd = $"SELECT productId, quantity FROM {tableName} WHERE userId = '{userId}'";
                var cmd = dataSource.CreateCommand(cartFetchCmd);
                await using (var reader = await cmd.ExecuteReaderAsync())
                {
                    while (await reader.ReadAsync())
                    {
                        CartItem item = new()
                        {
                            ProductId = reader.GetString(0),
                            Quantity = reader.GetInt32(1)
                        };
                        cart.Items.Add(item);
                    }
                }
                await Task.Run(() =>
                {
                    return cart;
                });
            }
            catch (Exception ex)
            {
                throw new Exception($"Unable to access cart storage due to an internal error. {ex}");
            }
            return cart;
        }


        public async Task EmptyCartAsync(string userId)
        {
            Console.WriteLine($"EmptyCartAsync called for userId={userId}");

            try
            {
                await using var dataSource = NpgsqlDataSource.Create(connectionString);
                var deleteCmd = $"DELETE FROM {tableName} WHERE userID = '{userId}'";
                await using (var cmd = dataSource.CreateCommand(deleteCmd))
                {
                    await Task.Run(() =>
                    {
                        return cmd.ExecuteNonQueryAsync();
                    });
                }
            }
            catch (Exception ex)
            {
                throw new Exception($"Unable to access cart storage due to an internal error. {ex}");
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
